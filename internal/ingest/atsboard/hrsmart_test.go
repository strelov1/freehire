package atsboard

import "testing"

func TestHRsmartPostingResolvesItsNativeTenant(t *testing.T) {
	source, board, _, ok := Recognize("https://gistworld.mua.hrdepartment.com/hrsmart/ats/Posting/view/3735?src=LinkedIn")
	if !ok || source != "hrsmart" || board != "gistworld.mua.hrdepartment.com" {
		t.Fatalf("recognized = %q/%q/%v, want native HRsmart tenant", source, board, ok)
	}
}
