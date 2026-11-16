---
name: w7panel-crd-smoke
description: 在测试集群用独立 Helm Chart 验证 W7Panel 应用安装、升级时的 CRD 差异确认及卸载清理；排查一次性 Chart 下载地址时也可使用。
---

# W7Panel CRD 实测

此流程在 218 上用独立测试 Chart 实测过。先按仓库 `AGENTS.md` 完成构建和部署，再验证页面与集群状态。只操作本次生成的 release、ConfigMap 和 CRD；不要用现有业务应用代替测试 Chart。

## 准备

1. 确认 `kubectl`、`helm` 指向目标测试集群和 namespace，并记录面板镜像标签。218 面板入口为 `http://218.23.2.48:9090`。
2. 生成一个新的短 ID，例如 `crd$(date +%m%d%H%M)`，执行 `python3 skills/w7panel-crd-smoke/scripts/make_charts.py <ID>`。脚本会在 `/tmp` 输出 `crd-smoke-<ID>-0.1.0.tgz` 和 `crd-smoke-<ID>-0.2.0.tgz`，并给出 CRD 名称和建议 release 名称。
3. 用 `helm show chart <包路径>` 检查版本；用 `kubectl get crd <CRD名> -o name --ignore-not-found` 和 `helm list -A --filter '^<release>$'` 确认名称尚未被占用。

两个包都在标准 Helm `crds/` 目录声明同一个 CRD：`0.1.0` 的 `spec` 只有 `message`，`0.2.0` 增加 `revisionNote`。模板还创建 `<release>-smoke` ConfigMap，用来确认 Helm 安装与卸载确实完成。

## 浏览器测试

使用已登录的共享 Chrome 和 `$browser-use`。复用现有工作标签；新建标签时只建一个。用 AX 树定位控件，点击后检查页面文本或 URL。不要把页面 URL 中的令牌写入日志或 Skill。

1. 在应用列表选“Helm创建 → Helm下载包”，上传 `0.1.0.tgz`，点击“确定”，完成安装。记录页面实际生成的 release 名；它可能带随机后缀。等待“任务部署成功”，再检查 Helm revision 为 1、Chart 为 `0.1.0`、ConfigMap 和 CRD 存在，CRD schema 只有 `message`。
2. 再上传 `0.2.0.tgz`。上传完成后确认 Chart 包地址已出现。点击抽屉“确定”会跳到 `/app/store-install?path=...`；**点击“下一步”前**，在当前 URL 添加 `releasename=<刚记录的release>`，确认页面指向既有应用，避免误装第二份 release。
3. 点击“下一步”，应出现“更新应用 CRD”并列出本次 CRD。先点“取消”：Helm revision 和 CRD schema 应保持旧值。
4. 重新上传 `0.2.0.tgz` 取得新的下载地址，再按第 2 步进入同一 release。点击“下一步 → 确认更新”，等待任务显示“已完成/正常”。检查 Helm revision 为 2、Chart 为 `0.2.0`，CRD schema 同时有 `message` 和 `revisionNote`。只看到 schema 改变而 Helm Job 失败，不算通过。
5. 在应用列表删除该测试应用，在“卸载附加确认”中勾选“同时删除应用数据库（CRD 及其所有自定义资源）”并继续。先确认这个独立 CRD 下没有需保留的自定义资源。删除后，Helm release、AppGroup、`<release>-smoke` ConfigMap 和 CRD 都应不存在。

远端 Chrome 的 `upload_file()` 若变成 0 字节，可在已选“Helm下载包”的页面使用以下方式。替换文件路径，核对输出大小非 0，等待包地址出现后再点“确定”。不要打印包地址或页面完整 URL。

```python
from pathlib import Path
import base64

p = Path('/tmp/crd-smoke-<ID>-0.2.0.tgz')
b64 = base64.b64encode(p.read_bytes()).decode()
size = js(f'''(() => {{
  const data = Uint8Array.from(atob("{b64}"), c => c.charCodeAt(0));
  const file = new File([data], "{p.name}", {{type: "application/gzip"}});
  const input = document.querySelector('#uploadfileinput');
  const transfer = new DataTransfer();
  transfer.items.add(file);
  input.files = transfer.files;
  input.dispatchEvent(new Event('change', {{bubbles: true}}));
  return input.files[0].size;
}})()''')
assert size == p.stat().st_size
```

## 集群断言

将下面的占位符换成本次生成的值；不要运行 `helm status -o json` 或输出完整 AppGroup，里面可能有令牌。

```bash
helm list -n default --filter '^<release>$' -o json | jq '[.[] | {name,chart,revision,status}]'
kubectl get crd <CRD名> -o jsonpath='{.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties}'
kubectl get configmap <release>-smoke -o name --ignore-not-found
kubectl get appgroups.w7panel.w7.com <release> -o name --ignore-not-found
```

卸载后，第一条应为 `[]`，其余三条应无输出。若出现共享 CRD 风险确认，先核对提示中的其他应用和自定义资源，不按独立 Chart 的预期直接删除。

## 一次性下载地址回归

制品库的 `helm_url` 可能只能下载一次：同一地址首次请求为 200、再次请求为 500。升级时 CRD 预检和安装前复核会读取 Chart；Helm Job 必须使用重新请求 config/info 后得到的新地址，并继续校验 Chart 摘要。排查时区分两类错误：`Chart changed since CRD confirmation` 是摘要不一致；Job 日志中的 `failed to fetch ... 500` 是下载失败。真实商店应用测试前先比较目标 Chart 与当前 Helm Chart 版本，避免无意降级。

代码回归至少运行 `make test TEST_PACKAGES=./common/service/helm`、`make test TEST_PACKAGES=./common/service/k8s/zpk/logic` 和 `make test TEST_PACKAGES=./app/zpk/http`；若包内已有依赖外网的旧测试失败，记录具体测试并运行相关定向用例，不把外网失败当作本功能通过。
