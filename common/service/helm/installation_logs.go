package helm

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	"unicode"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/kube"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

const installationLogDrainTimeout = 5 * time.Second

// Embed the concrete Helm client so optional interfaces (deletion propagation,
// three-way merge, logs, etc.) retain their original behavior.
type installationLogClient struct {
	*kube.Client
	logs *installationJobLogs
}

func (h *Helm) collectInstallationLogs(ctx context.Context, cfg *action.Configuration, namespace string) func() {
	if h.installationLog == nil {
		return func() {}
	}
	client, ok := cfg.KubeClient.(*kube.Client)
	if !ok {
		return func() {}
	}
	if namespace == "" {
		namespace = h.sdk.GetNamespace()
	}
	logs := newInstallationJobLogs(ctx, h.sdk.ClientSet, namespace, h.installationLog)
	cfg.KubeClient = &installationLogClient{Client: client, logs: logs}
	return logs.finish
}

func (c *installationLogClient) Create(resources kube.ResourceList) (*kube.Result, error) {
	result, err := c.Client.Create(resources)
	// Create can partly succeed. Its resource Infos are refreshed with server
	// UIDs; even on error, capture the Jobs that were actually created.
	c.logs.register(resources)
	return result, err
}

func (c *installationLogClient) Update(original, target kube.ResourceList, force bool) (*kube.Result, error) {
	result, err := c.Client.Update(original, target, force)
	if result != nil {
		c.logs.register(result.Created)
	}
	return result, err
}

func (c *installationLogClient) UpdateThreeWayMerge(original, target kube.ResourceList, force bool) (*kube.Result, error) {
	result, err := c.Client.UpdateThreeWayMerge(original, target, force)
	if result != nil {
		c.logs.register(result.Created)
	}
	return result, err
}

func (c *installationLogClient) WatchUntilReady(resources kube.ResourceList, timeout time.Duration) error {
	err := c.Client.WatchUntilReady(resources, timeout)
	// Helm deletes successful/failed hooks immediately after this method.
	// Drain their final lines while the Pods still exist, preserving Helm's error.
	c.logs.drain(resources)
	return err
}

type installationJobLogs struct {
	ctx       context.Context
	cancel    context.CancelFunc
	client    kubernetes.Interface
	namespace string
	output    io.Writer
	mu        sync.Mutex
	jobs      map[types.UID]chan struct{}
	outputMu  sync.Mutex
}

func newInstallationJobLogs(ctx context.Context, client kubernetes.Interface, namespace string, output io.Writer) *installationJobLogs {
	ctx, cancel := context.WithCancel(ctx)
	return &installationJobLogs{ctx: ctx, cancel: cancel, client: client, namespace: namespace, output: output, jobs: make(map[types.UID]chan struct{})}
}

func (l *installationJobLogs) write(source, message string) {
	l.outputMu.Lock()
	defer l.outputMu.Unlock()
	fmt.Fprintf(l.output, "[%s] %s\n", source, message)
}

func installationJobTitle(job metav1.Object) string {
	title := strings.TrimSpace(job.GetAnnotations()["w7.cc/title"])
	if title == "" {
		title = job.GetName()
	}
	title = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, title))
	if title == "" {
		return job.GetName()
	}
	return title
}

func (l *installationJobLogs) register(resources kube.ResourceList) {
	for _, info := range resources {
		if info == nil || info.Object == nil || info.Mapping == nil || info.Mapping.GroupVersionKind.Group != "batch" || info.Mapping.GroupVersionKind.Kind != "Job" {
			continue
		}
		obj, err := meta.Accessor(info.Object)
		if err != nil || obj.GetUID() == "" {
			continue
		}
		l.mu.Lock()
		if _, exists := l.jobs[obj.GetUID()]; exists {
			l.mu.Unlock()
			continue
		}
		done := make(chan struct{})
		l.jobs[obj.GetUID()] = done
		l.mu.Unlock()
		namespace := obj.GetNamespace()
		if namespace == "" {
			namespace = l.namespace
		}
		go func() {
			defer close(done)
			l.collectJob(namespace, obj.GetName(), obj.GetUID(), installationJobTitle(obj))
		}()
	}
}

func (l *installationJobLogs) drain(resources kube.ResourceList) {
	var pending []<-chan struct{}
	l.mu.Lock()
	for _, info := range resources {
		if info == nil || info.Object == nil {
			continue
		}
		if obj, err := meta.Accessor(info.Object); err == nil {
			if done := l.jobs[obj.GetUID()]; done != nil {
				pending = append(pending, done)
			}
		}
	}
	l.mu.Unlock()
	l.wait(pending)
}

func (l *installationJobLogs) wait(pending []<-chan struct{}) {
	timer := time.NewTimer(installationLogDrainTimeout)
	defer timer.Stop()
	for _, done := range pending {
		select {
		case <-done:
		case <-l.ctx.Done():
			return
		case <-timer.C:
			l.write("安装日志", "部分 Job 日志尚未收集完毕，可到任务详情查看")
			return
		}
	}
}

func (l *installationJobLogs) finish() {
	l.mu.Lock()
	pending := make([]<-chan struct{}, 0, len(l.jobs))
	for _, done := range l.jobs {
		pending = append(pending, done)
	}
	l.mu.Unlock()
	l.wait(pending)
	l.cancel()
	// Cancellation closes HTTP log streams and joins every collector before
	// helmgo exits, including its os.Exit error path.
	for _, done := range pending {
		<-done
	}
}

func (l *installationJobLogs) collectJob(namespace, name string, uid types.UID, title string) {
	ctx, cancel := context.WithCancel(l.ctx)
	var streams sync.WaitGroup
	defer func() {
		cancel()
		streams.Wait()
	}()
	seen := make(map[string]bool)
	announced := false
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		requestCtx, requestCancel := context.WithTimeout(ctx, 5*time.Second)
		job, err := l.client.BatchV1().Jobs(namespace).Get(requestCtx, name, metav1.GetOptions{})
		requestCancel()
		if err != nil || job.UID != uid {
			if ctx.Err() == nil {
				l.write(title, "无法读取本次 Job，日志可能不完整")
			}
			return
		}
		// Manual scripts are suspended resources, not installation work.
		if job.Spec.Suspend != nil && *job.Spec.Suspend {
			return
		}
		if !announced {
			l.write(title, "开始收集任务日志")
			announced = true
		}
		requestCtx, requestCancel = context.WithTimeout(ctx, 5*time.Second)
		pods, err := l.client.CoreV1().Pods(namespace).List(requestCtx, metav1.ListOptions{LabelSelector: "batch.kubernetes.io/controller-uid=" + string(uid)})
		requestCancel()
		if err != nil {
			if ctx.Err() == nil {
				l.write(title, "无法读取任务 Pod，日志可能不完整")
			}
			return
		}
		for _, pod := range pods.Items {
			owner := metav1.GetControllerOf(&pod)
			if owner == nil || owner.Kind != "Job" || owner.UID != uid {
				continue
			}
			statuses := append(append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...), pod.Status.ContainerStatuses...)
			for _, status := range statuses {
				if status.State.Running == nil && status.State.Terminated == nil {
					continue
				}
				key := string(pod.UID) + "/" + status.Name
				if seen[key] {
					continue
				}
				seen[key] = true
				streams.Add(1)
				go func(podName, container string) {
					defer streams.Done()
					l.collectContainer(ctx, namespace, podName, container, title)
				}(pod.Name, status.Name)
			}
		}
		for _, condition := range job.Status.Conditions {
			if condition.Status == corev1.ConditionTrue && (condition.Type == batchv1.JobComplete || condition.Type == batchv1.JobFailed) {
				streams.Wait()
				if ctx.Err() == nil {
					if condition.Type == batchv1.JobFailed {
						l.write(title, "任务失败："+condition.Reason)
					} else {
						l.write(title, "任务完成")
					}
				}
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (l *installationJobLogs) collectContainer(ctx context.Context, namespace, pod, container, title string) {
	source := title + "/" + pod + "/" + container
	cursor := installationLogCursor{}
	failures := 0
	for ctx.Err() == nil {
		options := &corev1.PodLogOptions{Container: container, Follow: true, Timestamps: true}
		if !cursor.second.IsZero() {
			since := metav1.NewTime(cursor.second)
			options.SinceTime = &since
		}
		stream, err := l.client.CoreV1().Pods(namespace).GetLogs(pod, options).Stream(ctx)
		if err == nil {
			err = cursor.copy(stream, func(line string) { l.write(source, line) })
			stream.Close()
		}
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			failures++
			if failures >= 3 || apierrors.IsForbidden(err) || apierrors.IsNotFound(err) {
				l.write(source, "日志读取失败，部分日志可能缺失")
				return
			}
		} else {
			failures = 0
		}
		// A finished container's stream reaches EOF normally. Otherwise reconnect
		// from the last second, suppressing the replayed records.
		requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		current, getErr := l.client.CoreV1().Pods(namespace).Get(requestCtx, pod, metav1.GetOptions{})
		cancel()
		if getErr != nil {
			l.write(source, "日志读取中断，部分日志可能缺失")
			return
		}
		statuses := append(current.Status.InitContainerStatuses, current.Status.ContainerStatuses...)
		for _, status := range statuses {
			if status.Name == container && status.State.Terminated != nil && err == nil {
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

// Kubernetes may serialize sinceTime to whole seconds. Keep a multiset for the
// last second so reconnects neither replay lines nor drop identical new lines.
type installationLogCursor struct {
	second time.Time
	seen   map[string]int
}

func (c *installationLogCursor) copy(input io.Reader, write func(string)) error {
	replay := make(map[string]int, len(c.seen))
	for key, count := range c.seen {
		replay[key] = count
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		stamp, _, ok := strings.Cut(line, " ")
		parsed, err := time.Parse(time.RFC3339Nano, stamp)
		if ok && err == nil {
			second := parsed.Truncate(time.Second)
			if second.Before(c.second) {
				continue
			}
			if second.After(c.second) {
				c.second, c.seen = second, make(map[string]int)
			}
			if replay[line] > 0 {
				replay[line]--
				continue
			}
			c.seen[line]++
		}
		write(line)
	}
	return scanner.Err()
}
