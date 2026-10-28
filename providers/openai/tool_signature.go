package openai

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// 仅按最终目标地址识别官方 OpenAI。OpenAI 格式也用于 Google 和未知兼容站，
// 不能按输入协议、模型名称或共享 provider 类型删除它们需要的扩展字段。
// 在全部参数合并后处理最终请求，不修改客户端历史或可供渠道重试的请求对象。
func stripGoogleSignatureForOfficialOpenAI(req *http.Request) error {
	if req == nil || req.URL == nil || !strings.EqualFold(req.URL.Hostname(), "api.openai.com") || req.Body == nil {
		return nil
	}
	body, err := io.ReadAll(req.Body)
	req.Body.Close()
	if err != nil {
		return err
	}
	for i, message := range gjson.GetBytes(body, "messages").Array() {
		for j, tool := range message.Get("tool_calls").Array() {
			if !tool.Get("extra_content.google.thought_signature").Exists() {
				continue
			}
			path := fmt.Sprintf("messages.%d.tool_calls.%d.extra_content", i, j)
			body, err = sjson.DeleteBytes(body, path+".google.thought_signature")
			if err != nil {
				return err
			}
			// 仅清理删除签名后变空的容器，保留其他厂商或 Google 扩展。
			if google := gjson.GetBytes(body, path+".google"); google.IsObject() && len(google.Map()) == 0 {
				body, err = sjson.DeleteBytes(body, path+".google")
				if err != nil {
					return err
				}
			}
			if extra := gjson.GetBytes(body, path); extra.IsObject() && len(extra.Map()) == 0 {
				body, err = sjson.DeleteBytes(body, path)
				if err != nil {
					return err
				}
			}
		}
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	return nil
}
