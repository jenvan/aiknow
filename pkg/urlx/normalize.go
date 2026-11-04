// Package urlx 提供 URL 规范化，用于「同一篇文章不要被当成两篇」。
//
// 核心场景是微信公众号：用户从微信分享出来的链接会带一串**每次分享都不同**的
// 参数（mpshare、srcid、sharer_shareinfo…），甚至带 #rd 片段。若直接按字符串
// 比较，同一篇文章转发两次就会重复入库。
package urlx

import (
	"net/url"
	"strings"
)

// dropParams 是纯跟踪/分享态参数，不参与文章身份判定。
//
// 公众号：mpshare / srcid / sharer_shareinfo / sharer_shareinfo_first 每次分享都变；
// scene / ascene / subscene / clicktime / sessionid 是渠道来源；
// 其余是常见的营销跟踪参数。
var dropParams = map[string]bool{
	// 公众号分享态
	"mpshare":                true,
	"mpshare_scene":          true,
	"srcid":                  true,
	"sharer_shareinfo":       true,
	"sharer_shareinfo_first": true,
	"subscene":               true,
	"sessionid":              true,
	"ascene":                 true,
	"devicetype":             true,
	"version":                true,
	"nettype":                true,
	"lang":                   true,
	"fontscale":              true,
	"realreporttime":         true,
	// 通用跟踪
	"scene":          true,
	"from":           true,
	"isappinstalled": true,
	"clicktime":      true,
	"enterid":        true,
	"exportkey":      true,
	"pass_ticket":    true,
	"wx_header":      true,
	"spm":            true,
	"share_token":    true,
	"utm_source":     true,
	"utm_medium":     true,
	"utm_campaign":   true,
	"utm_term":       true,
	"utm_content":    true,
}

// Normalize 规范化 URL：去片段、剔除跟踪参数、query 排序。
//
// 无法解析（或非 http/https）时原样返回，交由采集侧的 SSRF 校验负责拒绝——
// 本函数只做去重归一，不做安全判断。
func Normalize(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return raw
	}

	u.Fragment = ""
	u.RawFragment = ""

	q := u.Query()
	for k := range q {
		if dropParams[strings.ToLower(k)] {
			q.Del(k)
		}
	}

	// 公众号文章的身份由 __biz + mid + idx + sn 决定（chksm 是内容校验签名，
	// 同一篇稳定，保留以兼容可能的风控校验）。排序只为让两次归一结果逐字相等。
	if strings.EqualFold(u.Host, "mp.weixin.qq.com") && q.Get("__biz") != "" {
		keys := []string{"__biz", "mid", "idx", "sn", "chksm"}
		ordered := url.Values{}
		for _, k := range keys {
			if v := q.Get(k); v != "" {
				ordered.Set(k, v)
			}
		}
		q = ordered
	}

	u.RawQuery = q.Encode()
	return u.String()
}
