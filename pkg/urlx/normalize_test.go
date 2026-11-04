package urlx

import "testing"

func TestNormalizeWeixinShare(t *testing.T) {
	// 真实分享链接：同一篇转发两次，mpshare/srcid/sharer_shareinfo 都不同
	in := "https://mp.weixin.qq.com/s?__biz=MzY0MDk2Mzg5MA==&mid=2247486575&idx=1" +
		"&sn=ed93c66728f6b1cfd5b5f1670bbb2c80&chksm=f0409de9e53f90268314b40e20be4b" +
		"&mpshare=1&scene=1&srcid=1104XYVYOOlNfCX9el7foS7N" +
		"&sharer_shareinfo=357449aee351f22c50df8eb5008f5b88" +
		"&sharer_shareinfo_first=357449aee351f22c50df8eb5008f5b88#rd"
	want := "https://mp.weixin.qq.com/s?__biz=MzY0MDk2Mzg5MA%3D%3D&chksm=" +
		"f0409de9e53f90268314b40e20be4b&idx=1&mid=2247486575&sn=ed93c66728f6b1cfd5b5f1670bbb2c80"

	if got := Normalize(in); got != want {
		t.Fatalf("Normalize() =\n  %s\nwant\n  %s", got, want)
	}
}

func TestNormalizeWeixinShareStable(t *testing.T) {
	// 两次分享只有分享态参数不同 → 归一后必须逐字相等（去重的前提）
	a := "https://mp.weixin.qq.com/s?__biz=MQ==&mid=1&idx=1&sn=abc&mpshare=1&scene=1&srcid=AAA#rd"
	b := "https://mp.weixin.qq.com/s?__biz=MQ==&mid=1&idx=1&sn=abc&mpshare=1&scene=1&srcid=BBB&sharer_shareinfo=CCC"
	if Normalize(a) != Normalize(b) {
		t.Fatalf("share params not stripped:\n  %s\n  %s", Normalize(a), Normalize(b))
	}
}

func TestNormalizeGenericTracking(t *testing.T) {
	cases := map[string]string{
		"https://example.com/a?utm_source=x&utm_medium=y#top": "https://example.com/a",
		"https://example.com/a?b=2&a=1":                       "https://example.com/a?a=1&b=2",
		"https://example.com/a?from=timeline&id=3":            "https://example.com/a?id=3",
		"https://example.com/path#frag":                       "https://example.com/path",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeShortWeixinLink(t *testing.T) {
	// /s/<token> 短链的身份就是 path，query 一律去掉
	in := "https://mp.weixin.qq.com/s/dmwT6eKnMluk8DfXzpPucA?scene=21&from=timeline"
	want := "https://mp.weixin.qq.com/s/dmwT6eKnMluk8DfXzpPucA"
	if got := Normalize(in); got != want {
		t.Fatalf("Normalize() = %q, want %q", got, want)
	}
}

func TestNormalizeInvalid(t *testing.T) {
	for _, raw := range []string{"", "not a url", "ftp://example.com/a", "javascript:alert(1)"} {
		if got := Normalize(raw); got != raw {
			t.Errorf("Normalize(%q) = %q, want unchanged", raw, got)
		}
	}
}
