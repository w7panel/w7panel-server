package controller

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/remotecommand"
	"github.com/w7panel/w7panel/common/service/k8s/shell"
	"github.com/w7panel/w7panel/common/service/k8s/terminal"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	remotecommand2 "k8s.io/client-go/tools/remotecommand"
	"k8s.io/kubectl/pkg/cmd/cp"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

const LS = `ls -l -AF /proc/76008/root/w7panel | awk -v passwd="/proc/76008/root/etc/passwd" -v group="/proc/76008/root/etc/group" '
BEGIN {
    while ((getline < passwd) > 0) {
        split($0, fields, ":");
        uid_to_user[fields[3]] = fields[1];
    }
    close(passwd);
    while ((getline < group) > 0) {
        split($0, fields, ":");
        gid_to_group[fields[3]] = fields[1];
    }
    close(group);
}
{
    uid = $3;
    gid = $4;
    user = (uid in uid_to_user)? uid_to_user[uid] : uid;
    group = (gid in gid_to_group)? gid_to_group[gid] : gid;
    $3 = user;
    $4 = group;
    print;
}'`

func lsProxy(pid string) string {
	result := strings.ReplaceAll(LS, "76008", pid)
	result = strings.ReplaceAll(result, "ls -l -AF /proc/76008/root/", "")
	return result
}

type PodExec struct {
	controller.Abstract
}

func (self PodExec) getExecClient(token string, podName string) (*k8s.Sdk, error) {
	client, err := k8s.NewK8sClient().Channel(token)
	if err != nil {
		return nil, err
	}
	if strings.Contains(podName, "w7panel-agent") {
		return k8s.NewK8sClient().Sdk, nil
	}
	return client, nil
}

func (self PodExec) Exec(http *gin.Context) {
	type ParamsValidate struct {
		Namespace     string   `form:"namespace" binding:"required"`
		PodName       string   `form:"podName" binding:"required"`
		ContainerName string   `form:"containerName" binding:"required"`
		Command       []string `form:"command" binding:"required"`
		Tty           bool     `form:"tty"`
	}
	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}

	r := http.Request
	w := http.Writer

	var conn *websocket.Conn
	var err error
	if websocket.IsWebSocketUpgrade(r) {
		conn, err = upgrader.Upgrade(w, r, nil)
		if err != nil {
			self.JsonResponseWithServerError(http, err)
			return
		}
	}

	session := terminal.NewTerminalSession(conn)
	execTimeout := facade.GetConfig().GetInt("k8s.exec_timeout_seconds")
	if execTimeout <= 0 {
		execTimeout = 1800
	}
	ctx, cancel := context.WithTimeout(http.Request.Context(), time.Duration(execTimeout)*time.Second)
	defer cancel()
	session.SetContext(ctx)
	defer func() {
		reason := session.GetCloseReason()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			reason = "timeout"
		}
		slog.Info("pod exec session closing", "namespace", params.Namespace, "podName", params.PodName, "containerName", params.ContainerName, "reason", reason)
		session.CloseWithReason(reason)
	}()

	client, err := k8s.NewK8sClient().Channel(http.MustGet("k8s_token").(string))
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	cmd := params.Command
	// if params.Pid != "" && len(cmd) > 0 && cmd[0] == "ls" {
	// 	cmd = []string{"/bin/sh", "-c", lsProxy(params.Pid)}
	// }
	err = client.RunExec(session, params.Namespace, params.PodName, params.ContainerName, cmd, params.Tty)
	if err != nil {
		reason := "upstream_close"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			reason = "timeout"
		}
		session.CloseWithReason(reason)
		slog.Warn("pod exec run error", "namespace", params.Namespace, "podName", params.PodName, "containerName", params.ContainerName, "reason", reason, "err", err)
		self.JsonResponseWithServerError(http, err)
		return
	}
	if conn == nil {
		http.Writer.Write(session.GetWriterBytes())
	}
}

func (self PodExec) ExecAll(http *gin.Context) {
	type ParamsValidate struct {
		Namespace     string   `form:"namespace" json:"namespace" binding:"required"`
		PodNames      []string `form:"podNames" json:"podNames"`
		PodName       string   `form:"podName" json:"podName"`
		ContainerName string   `form:"containerName" json:"containerName" binding:"required"`
		Command       []string `form:"command" json:"command" binding:"required"`
		Tty           bool     `form:"tty" json:"tty"`
	}
	type ExecAllItem struct {
		PodName string `json:"podName"`
		Output  string `json:"output"`
		Success bool   `json:"success"`
		Error   string `json:"error,omitempty"`
	}

	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}
	if len(params.PodNames) == 0 && params.PodName != "" {
		params.PodNames = []string{params.PodName}
	}
	if len(params.PodNames) == 0 {
		self.JsonResponseWithServerError(http, errors.New("podNames is required"))
		return
	}
	if websocket.IsWebSocketUpgrade(http.Request) {
		self.JsonResponseWithServerError(http, errors.New("exec all does not support websocket"))
		return
	}

	token := http.MustGet("k8s_token").(string)
	execTimeout := facade.GetConfig().GetInt("k8s.exec_timeout_seconds")
	if execTimeout <= 0 {
		execTimeout = 1800
	}
	ctx, cancel := context.WithTimeout(http.Request.Context(), time.Duration(execTimeout)*time.Second)
	defer cancel()

	results := make([]ExecAllItem, 0, len(params.PodNames))
	for _, podName := range params.PodNames {
		item := ExecAllItem{
			PodName: podName,
		}

		client, err := self.getExecClient(token, podName)
		if err != nil {
			item.Error = err.Error()
			results = append(results, item)
			continue
		}

		session := terminal.NewTerminalSession(nil)
		session.SetContext(ctx)
		err = client.RunExec(session, params.Namespace, podName, params.ContainerName, params.Command, params.Tty)
		item.Output = string(session.GetWriterBytes())
		if err != nil {
			item.Error = err.Error()
		} else {
			item.Success = true
		}
		results = append(results, item)
	}

	self.JsonResponseWithoutError(http, results)
}

func (p PodExec) NodeTty(http *gin.Context) {
	type ParamsValidate struct {
		Shell  string `form:"shell,default=/bin/bash" binding:"oneof=/bin/sh /bin/bash"`
		HostIp string `form:"hostIp" binding:"required"`
	}
	params := ParamsValidate{}
	if !p.Validate(http, &params) {
		return
	}

	conn, err := upgrader.Upgrade(http.Writer, http.Request, nil)
	if err != nil {
		p.JsonResponseWithServerError(http, err)
		return
	}

	token := http.MustGet("k8s_token").(string)
	k8sToken := k8s.NewK8sToken(token)
	session := terminal.NewTerminalSession(conn)
	execTimeout := facade.GetConfig().GetInt("k8s.exec_timeout_seconds")
	if execTimeout <= 0 {
		execTimeout = 1800
	}
	ctx, cancel := context.WithTimeout(http.Request.Context(), time.Duration(execTimeout)*time.Second)
	defer cancel()
	session.SetContext(ctx)
	defer func() {
		reason := session.GetCloseReason()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			reason = "timeout"
		}
		slog.Info("node tty session closing", "hostIp", params.HostIp, "reason", reason)
		session.CloseWithReason(reason)
	}()
	rootsdk := k8s.NewK8sClient().Sdk
	var findPod *corev1.Pod
	shells := []string{params.Shell}
	if k8sToken.IsK3kCluster() {
		// client, err := k8s.NewK8sClient().ChannelLocal(http.MustGet("k8s_token").(string), true)
		k3kConfig, err := k8sToken.GetK3kConfig()
		if err != nil {
			p.JsonResponseWithServerError(http, err)
			return
		}
		pods, err := rootsdk.ClientSet.CoreV1().Pods(k3kConfig.Namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "cluster"})
		if err != nil {
			p.JsonResponseWithServerError(http, err)
			return
		}
		for _, pod := range pods.Items {
			if pod.Status.PodIP == params.HostIp {
				findPod = &pod
			}
		}
	} else {
		findPod, err = rootsdk.GetDaemonsetAgentPod(rootsdk.GetNamespace(), params.HostIp)
		if err != nil {
			p.JsonResponseWithServerError(http, err)
			return
		}
		shells = []string{"nsenter", "-t", "1", "--mount", "--uts", "--ipc", "--net", "--pid", "--", params.Shell}
	}
	if findPod == nil {
		p.JsonResponseWithServerError(http, fmt.Errorf("not found agent pod for hostIp: %s", params.HostIp))
		return
	}
	err = rootsdk.RunExec(session, findPod.Namespace, findPod.Name, findPod.Spec.Containers[0].Name, shells, true)
	if err != nil {
		reason := "upstream_close"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			reason = "timeout"
		}
		session.CloseWithReason(reason)
		slog.Warn("node tty run error", "hostIp", params.HostIp, "reason", reason, "err", err)
		p.JsonResponseWithServerError(http, err)
		return
	}
}

func (self PodExec) Tty(http *gin.Context) {
	type ParamsValidate struct {
		Shell string `form:"shell,default=/bin/bash" binding:"oneof=/bin/sh /bin/bash"`
	}
	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}

	ttyChrootDir := facade.GetConfig().GetString("k8s.tty_chroot_dir")
	cmd := exec.Command(params.Shell)
	// 获取当前进程的所有环境变量
	cmd.Env = os.Environ()
	// 设置新的环境变量
	cmd.Env = append(cmd.Env, "TERM=xterm-256color")

	if ttyChrootDir != "" {
		// cmd.Dir = "ttytmp" //*ttyChrootDir 不能用 /
		// cmd.Env = []string{
		// 	"TERM=xterm-256color",
		// 	"KUBERNETES_TOKEN=" + config.BearerToken,
		// 	"KUBERNETES_SERVICE_HOST=" + os.Getenv("KUBERNETES_SERVICE_HOST"),
		// 	"KUBERNETES_SERVICE_PORT=" + os.Getenv("KUBERNETES_SERVICE_PORT"),
		// 	"KUBERNETES_CAFILE=" + "/.kube/ca.crt",
		// 	"HOME=" + os.Getenv("HOME"),
		// }
	}
	// cmd.SysProcAttr = &syscall.SysProcAttr{
	// 	Chroot: ttyChrootDir,
	// }

	conn, err := upgrader.Upgrade(http.Writer, http.Request, nil)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}

	session := terminal.NewTerminalSession(conn)
	execTimeout := facade.GetConfig().GetInt("k8s.exec_timeout_seconds")
	if execTimeout <= 0 {
		execTimeout = 1800
	}
	ctx, cancel := context.WithTimeout(http.Request.Context(), time.Duration(execTimeout)*time.Second)
	defer cancel()
	session.SetContext(ctx)
	defer func() {
		reason := session.GetCloseReason()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			reason = "timeout"
		}
		slog.Info("tty session closing", "reason", reason)
		session.CloseWithReason(reason)
	}()
	token := http.MustGet("k8s_token").(string)
	k8sToken := k8s.NewK8sToken(token)
	if k8sToken.IsK3kCluster() {
		// client, err := k8s.NewK8sClient().ChannelLocal(http.MustGet("k8s_token").(string), true)
		client := k8s.NewK8sClient()

		k3kConfig, err := k8sToken.GetK3kConfig()
		if err != nil {
			self.JsonResponseWithServerError(http, err)
			return
		}
		// clientsdk, err := client.Channel(token)
		// if err != nil {
		// 	self.JsonResponseWithServerError(http, err)
		// 	return
		// }
		params.Shell = "/bin/sh" //k3k pod 只支持 /bin/sh
		err = client.RunExec(session, k3kConfig.Namespace, k3kConfig.GetK3kServer0Name(), k3kConfig.GetK3kServer0ContainerName(), []string{params.Shell}, true)
		if err != nil {
			reason := "upstream_close"
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				reason = "timeout"
			}
			session.CloseWithReason(reason)
			slog.Warn("tty k3k run error", "reason", reason, "err", err)
			self.JsonResponseWithServerError(http, err)
			return
		}
	} else {
		err = remotecommand.NewLocalExecutor(cmd).StreamWithContext(session.Context(), remotecommand2.StreamOptions{
			Stdin:             session,
			Stdout:            session,
			Stderr:            session,
			Tty:               true,
			TerminalSizeQueue: session,
		})
		if err != nil {
			reason := "upstream_close"
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				reason = "timeout"
			}
			session.CloseWithReason(reason)
			slog.Error("tty error", "reason", reason, "err", err)
			return
		}
	}

}

// no test pass
func (self PodExec) KubectlCp(http *gin.Context) {
	baseDir := facade.Config.GetString("s3.base_dir")
	// token := http.MustGet("k8s_token").(string)
	type ParamsValidate struct {
		From      string `form:"from"      binding:"required"`
		To        string `form:"to"        binding:"required"`
		Namespace string `form:"namespace" binding:"required"`
		Upload    string `form:"upload"  binding:"required"`
		Podname   string `form:"podName" binding:"required"`
	}

	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}

	if params.Upload == "1" {
		params.From = filepath.Join(baseDir, params.From)
		params.To = params.Podname + ":" + params.To
	} else {
		params.To = filepath.Join(baseDir, params.To)
		params.From = params.Podname + ":" + params.From
	}

	rootSdk := k8s.NewK8sClient().Sdk
	token := http.MustGet("k8s_token").(string)
	client, err := k8s.NewK8sClient().Channel(http.MustGet("k8s_token").(string))
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	k8stoken := k8s.NewK8sToken(token)
	if k8stoken.IsK3kCluster() {
		client = rootSdk
	}
	cmdutil.BehaviorOnFatal(func(errstr string, code int) {

		self.JsonResponseWithServerError(http, fmt.Errorf("%v", errstr))

		// if err := recover(); err != nil {
		// 	self.JsonResponseWithServerError(http, fmt.Errorf("%v", err))
		// }
		http.Abort()
	})
	factory := cmdutil.NewFactory(client.PodExecClient())
	cmd := cp.NewCmdCp(factory, genericiooptions.NewTestIOStreamsDiscard())
	cmd.SetArgs([]string{params.From, params.To})
	err = cmd.Execute()
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonSuccessResponse(http)

}

// 根据pod获取pid
func (self PodExec) GetNodePid(http *gin.Context) {

	token := http.MustGet("k8s_token").(string)
	k8sToken := k8s.NewK8sToken(token)
	if !k8sToken.IsK3kCluster() {
		self.JsonResponse(http, gin.H{
			"pid": 1,
		}, nil, 200)
		return
	}
	type VParam struct {
		Namespace string `form:"namespace" binding:"required"`
		PodName   string `form:"podName" binding:"required"`
	}
	params := VParam{}
	if !self.Validate(http, &params) {
		return
	}

	sdk := k8s.NewK8sClient().Sdk
	pod, err := sdk.ClientSet.CoreV1().Pods(k8sToken.GetNamespace()).Get(context.TODO(), params.PodName, metav1.GetOptions{})
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	agentPod, err := sdk.GetDaemonsetAgentPod("default", pod.Status.HostIP)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	pid, err := shell.GetPid(agentPod, pod.Status.ContainerStatuses[0].ContainerID, true, sdk)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonResponse(http, gin.H{
		"pid": pid,
	}, nil, 200)
}
