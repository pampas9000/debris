package kit

import "testing"

func TestExtractPinyin(t *testing.T) {
	cases := []struct {
		name         string
		wantPinyin   string
		wantCategory string
	}{
		{name: "卡通", wantPinyin: "KaTong", wantCategory: "K"},
		{name: "风景", wantPinyin: "FengJing", wantCategory: "F"},
		{name: "标签组 1", wantPinyin: "BiaoQianZu 1", wantCategory: "B"},
		{name: "UI设计", wantPinyin: "UISheJi", wantCategory: "U"},
		{name: "landscape", wantPinyin: "landscape", wantCategory: "L"},
		{name: "Design", wantPinyin: "Design", wantCategory: "D"},
		{name: "  alpha", wantPinyin: "  alpha", wantCategory: "A"},
		{name: "123数字", wantPinyin: "123ShuZi", wantCategory: "#"},
		{name: "_special", wantPinyin: "_special", wantCategory: "#"},
		{name: "", wantPinyin: "", wantCategory: "#"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotPinyin, gotCategory := ExtractPinyin(tc.name)
			if gotPinyin != tc.wantPinyin {
				t.Fatalf("ExtractPinyin(%q) pinyin = %q, want %q", tc.name, gotPinyin, tc.wantPinyin)
			}
			if gotCategory != tc.wantCategory {
				t.Fatalf("ExtractPinyin(%q) category = %q, want %q", tc.name, gotCategory, tc.wantCategory)
			}
		})
	}
}
