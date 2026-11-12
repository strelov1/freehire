package handler

import "testing"

// A stored CV is served as what it is: a PDF upload as a PDF, pasted text (the website's paste
// path, add_cv) as text — never text labelled as a PDF a viewer cannot open.
func TestUploadedCVType(t *testing.T) {
	for _, tc := range []struct {
		data         string
		wantType     string
		wantFilename string
	}{
		{"%PDF-1.7\n...", "application/pdf", "cv.pdf"},
		{"Jane Doe\nBackend engineer", "text/plain; charset=utf-8", "cv.txt"},
		{"", "text/plain; charset=utf-8", "cv.txt"},
	} {
		gotType, gotName := uploadedCVType([]byte(tc.data))
		if gotType != tc.wantType || gotName != tc.wantFilename {
			t.Errorf("uploadedCVType(%q) = %q, %q; want %q, %q", tc.data, gotType, gotName, tc.wantType, tc.wantFilename)
		}
	}
}
