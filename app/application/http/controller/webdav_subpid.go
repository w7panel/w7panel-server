package controller

import (
	"encoding/xml"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/procpath"
)

type davEntry struct {
	Name    string
	Type    string
	Size    string
	MTime   string
	UID     string
	GID     string
	Mode    string
	ReqPath string
}

func (c Webdav) handleSubPid(ctx *gin.Context, pid, subpid, prefix string) {
	reqPath := strings.TrimPrefix(ctx.Request.URL.Path, prefix)
	cleanPath, err := procpath.CleanContainerPath(reqPath)
	if err != nil {
		ctx.String(http.StatusBadRequest, err.Error())
		return
	}

	switch ctx.Request.Method {
	case http.MethodOptions:
		ctx.Header("Allow", "OPTIONS, PROPFIND, GET, HEAD, PUT, DELETE, MKCOL, MOVE, COPY")
		ctx.Status(http.StatusOK)
	case "PROPFIND":
		c.subPidPropfind(ctx, pid, subpid, prefix, cleanPath)
	case http.MethodGet, http.MethodHead:
		c.subPidGet(ctx, pid, subpid, cleanPath)
	case http.MethodPut:
		c.subPidPut(ctx, pid, subpid, cleanPath)
	case http.MethodDelete:
		c.subPidExecStatus(ctx, pid, subpid, cleanPath, func(target string) string {
			return "rm -rf -- " + procpath.ShellQuote(target)
		})
	case "MKCOL":
		c.subPidExecStatus(ctx, pid, subpid, cleanPath, func(target string) string {
			return "mkdir -p -- " + procpath.ShellQuote(target)
		})
	case "MOVE", "COPY":
		c.subPidCopyMove(ctx, pid, subpid, prefix, cleanPath, ctx.Request.Method == "MOVE")
	default:
		ctx.Status(http.StatusMethodNotAllowed)
	}
}

func (c Webdav) subPidPropfind(ctx *gin.Context, pid, subpid, prefix, cleanPath string) {
	depth := ctx.Request.Header.Get("Depth")
	entries, err := c.subPidList(pid, subpid, cleanPath, depth != "0")
	if err != nil {
		ctx.String(http.StatusNotFound, err.Error())
		return
	}
	ctx.Header("Content-Type", `application/xml; charset="utf-8"`)
	ctx.String(207, buildMultiStatus(prefix, entries))
}

func (c Webdav) subPidList(pid, subpid, cleanPath string, includeChildren bool) ([]davEntry, error) {
	target, err := procpath.InnerFilePath(subpid, cleanPath)
	if err != nil {
		return nil, err
	}
	script := `
set -e
p=` + procpath.ShellQuote(target) + `
if [ ! -e "$p" ] && [ ! -L "$p" ]; then exit 2; fi
stat -c '.__self__	%F	%s	%Y	%u	%g	%a' "$p"
if [ -d "$p" ] && [ ` + strconv.FormatBool(includeChildren) + ` = true ]; then
  find "$p" -mindepth 1 -maxdepth 1 -printf '%f	%y	%s	%T@	%U	%G	%m\n'
fi
`
	out, err := procpath.RunInPidNamespace(pid, script)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	entries := make([]davEntry, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 7 {
			continue
		}
		name := parts[0]
		reqPath := cleanPath
		typeName := normalizeDavType(parts[1])
		if name != ".__self__" {
			reqPath = path.Join(cleanPath, name)
			if strings.HasSuffix(cleanPath, "/") {
				reqPath = cleanPath + name
			}
			typeName = normalizeFindType(parts[1])
		} else if cleanPath == "/" {
			name = ""
		} else {
			name = path.Base(cleanPath)
		}
		entries = append(entries, davEntry{
			Name:    name,
			Type:    typeName,
			Size:    parts[2],
			MTime:   parts[3],
			UID:     parts[4],
			GID:     parts[5],
			Mode:    parts[6],
			ReqPath: reqPath,
		})
	}
	return entries, nil
}

func normalizeDavType(raw string) string {
	switch {
	case strings.Contains(raw, "directory"):
		return "directory"
	case strings.Contains(raw, "symbolic link"):
		return "symlink"
	case strings.Contains(raw, "regular"):
		return "file"
	case strings.Contains(raw, "fifo"):
		return "fifo"
	case strings.Contains(raw, "socket"):
		return "socket"
	case strings.Contains(raw, "device"):
		return "device"
	default:
		return "unknown"
	}
}

func normalizeFindType(raw string) string {
	switch raw {
	case "d":
		return "directory"
	case "l":
		return "symlink"
	case "f":
		return "file"
	case "p":
		return "fifo"
	case "s":
		return "socket"
	case "b", "c":
		return "device"
	default:
		return "unknown"
	}
}

func buildMultiStatus(prefix string, entries []davEntry) string {
	var b strings.Builder
	b.WriteString(xml.Header)
	b.WriteString(`<D:multistatus xmlns:D="DAV:" xmlns:w7panel="w7panel">`)
	for _, entry := range entries {
		href := prefix + encodeDavPath(entry.ReqPath)
		if entry.Type == "directory" && !strings.HasSuffix(href, "/") {
			href += "/"
		}
		b.WriteString(`<D:response><D:href>`)
		b.WriteString(html.EscapeString(href))
		b.WriteString(`</D:href><D:propstat><D:prop>`)
		b.WriteString(`<D:displayname>`)
		b.WriteString(html.EscapeString(entry.Name))
		b.WriteString(`</D:displayname>`)
		if entry.Type == "directory" {
			b.WriteString(`<D:resourcetype><D:collection/></D:resourcetype>`)
		} else {
			b.WriteString(`<D:resourcetype/>`)
			b.WriteString(`<D:getcontentlength>`)
			b.WriteString(html.EscapeString(entry.Size))
			b.WriteString(`</D:getcontentlength>`)
		}
		b.WriteString(`<D:getlastmodified>`)
		b.WriteString(html.EscapeString(formatDavTime(entry.MTime)))
		b.WriteString(`</D:getlastmodified>`)
		b.WriteString(`<w7panel:uid>`)
		b.WriteString(html.EscapeString(entry.UID))
		b.WriteString(`</w7panel:uid><w7panel:gid>`)
		b.WriteString(html.EscapeString(entry.GID))
		b.WriteString(`</w7panel:gid><w7panel:mode>`)
		b.WriteString(html.EscapeString(entry.Mode))
		b.WriteString(`</w7panel:mode><w7panel:type>`)
		b.WriteString(html.EscapeString(entry.Type))
		b.WriteString(`</w7panel:type><w7panel:editable>`)
		b.WriteString(strconv.FormatBool(entry.Type == "file" || entry.Type == "symlink"))
		b.WriteString(`</w7panel:editable>`)
		b.WriteString(`</D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response>`)
	}
	b.WriteString(`</D:multistatus>`)
	return b.String()
}

func encodeDavPath(p string) string {
	if p == "/" || p == "" {
		return "/"
	}
	parts := strings.Split(strings.TrimPrefix(p, "/"), "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return "/" + strings.Join(parts, "/")
}

func formatDavTime(raw string) string {
	raw = strings.Split(raw, ".")[0]
	sec, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return time.Now().UTC().Format(http.TimeFormat)
	}
	return time.Unix(sec, 0).UTC().Format(http.TimeFormat)
}

func (c Webdav) subPidGet(ctx *gin.Context, pid, subpid, cleanPath string) {
	target, err := procpath.InnerFilePath(subpid, cleanPath)
	if err != nil {
		ctx.String(http.StatusBadRequest, err.Error())
		return
	}
	script := "cat -- " + procpath.ShellQuote(target)
	out, err := procpath.RunInPidNamespace(pid, script)
	if err != nil {
		ctx.String(http.StatusNotFound, err.Error())
		return
	}
	ctx.Data(http.StatusOK, "application/octet-stream", out)
}

func (c Webdav) subPidPut(ctx *gin.Context, pid, subpid, cleanPath string) {
	target, err := procpath.InnerFilePath(subpid, cleanPath)
	if err != nil {
		ctx.String(http.StatusBadRequest, err.Error())
		return
	}
	script := "mkdir -p -- " + procpath.ShellQuote(path.Dir(target)) + " && cat > " + procpath.ShellQuote(target)
	if _, err := procpath.RunInPidNamespaceWithInput(pid, script, ctx.Request.Body); err != nil {
		ctx.String(http.StatusInternalServerError, err.Error())
		return
	}
	ctx.Status(http.StatusCreated)
}

func (c Webdav) subPidExecStatus(ctx *gin.Context, pid, subpid, cleanPath string, cmdPrefix func(string) string) {
	target, err := procpath.InnerFilePath(subpid, cleanPath)
	if err != nil {
		ctx.String(http.StatusBadRequest, err.Error())
		return
	}
	if _, err := procpath.RunInPidNamespace(pid, cmdPrefix(target)); err != nil {
		ctx.String(http.StatusInternalServerError, err.Error())
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (c Webdav) subPidCopyMove(ctx *gin.Context, pid, subpid, prefix, cleanPath string, move bool) {
	dstHeader := ctx.Request.Header.Get("Destination")
	if dstHeader == "" {
		ctx.String(http.StatusBadRequest, "missing Destination header")
		return
	}
	dstURL, err := url.Parse(dstHeader)
	if err != nil {
		ctx.String(http.StatusBadRequest, err.Error())
		return
	}
	dstReq, err := extractDavRequestPath(dstURL.Path, prefix)
	if err != nil {
		ctx.String(http.StatusBadRequest, err.Error())
		return
	}
	dstClean, err := procpath.CleanContainerPath(dstReq)
	if err != nil {
		ctx.String(http.StatusBadRequest, err.Error())
		return
	}
	src, err := procpath.InnerFilePath(subpid, cleanPath)
	if err != nil {
		ctx.String(http.StatusBadRequest, err.Error())
		return
	}
	dst, err := procpath.InnerFilePath(subpid, dstClean)
	if err != nil {
		ctx.String(http.StatusBadRequest, err.Error())
		return
	}
	op := "cp -a -- "
	if move {
		op = "mv -- "
	}
	script := "mkdir -p -- " + procpath.ShellQuote(path.Dir(dst)) + " && " + op + procpath.ShellQuote(src) + " " + procpath.ShellQuote(dst)
	if _, err := procpath.RunInPidNamespace(pid, script); err != nil {
		ctx.String(http.StatusInternalServerError, fmt.Sprintf("%v", err))
		return
	}
	ctx.Status(http.StatusCreated)
}

func extractDavRequestPath(urlPath, prefix string) (string, error) {
	if strings.HasPrefix(urlPath, prefix) {
		return strings.TrimPrefix(urlPath, prefix), nil
	}
	idx := strings.Index(urlPath, prefix)
	if idx >= 0 {
		return strings.TrimPrefix(urlPath[idx:], prefix), nil
	}
	return "", fmt.Errorf("destination does not match webdav prefix")
}
