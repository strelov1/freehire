package jobfacts

import "testing"

// TestViaRecruiter pins the recruiter-statement phrases against text lifted from live
// catalogue postings (sampled 2026-10-30), including the near-misses that sample showed
// are NOT a recruiter speaking: a product company's plural "our clients", a care
// provider's "confidential client information", a consultancy's "our client's needs".
func TestViaRecruiter(t *testing.T) {
	tests := []struct {
		name string
		desc string
		want bool
	}{
		{"on behalf of our client", "We are hiring on behalf of our client for the role of Swedish Customer Service Agent.", true},
		{"on behalf of a client", "Xpert Recruitment are recruiting on behalf of a client for a permanent Maintenance Technician.", true},
		{"leading on behalf of", "On behalf of our client, Procom Services is searching for a SharePoint Developer.", true},
		{"our client is a", "Our client is a venture-backed healthcare AI company improving access to care.", true},
		{"our client is looking", "Our client is looking for a hands-on Full Stack Software Developer.", true},
		{"our client is seeking", "Our client is seeking a talented Quantitative Developer.", true},
		{"our client comma a", "Our client, a leading organisation in the retail sector, is seeking a Programme Manager.", true},
		{"valued client", "Our valued client, a prominent player in African capital markets, is hiring.", true},
		{"about our client heading", "<h3>About Our Client</h3><p>A nationally recognized boutique firm.</p>", true},
		{"recruiting words on behalf of", "Raydar is recruiting for this role on behalf of our client.", true},
		{"client of ours", "A high profile international retail client of ours is looking for a Sr. Software Engineer.", true},
		{"join our client's team", "We are looking for a Senior Audit Manager to join our client’s team.", true},
		{"our customer is hiring", "Our customer is hiring for the following role: Sales Executive.", true},
		{"markup between words", "<p>Our <b>client</b> is a fast-growing fintech.</p>", true},

		{"plural clients", "We help our clients ship faster. Our clients, a diverse group, love us.", false},
		{"confidential client information", "Maintain accurate and confidential client billing records.", false},
		{"consultancy needs", "On-site presence may change depending on our client's needs.", false},
		{"for a client caregiver", "Experience in caring for a client or a family member.", false},
		{"client base", "We manage approximately $80bn for a client base of institutions.", false},
		{"silent", "We are hiring a backend engineer to work on our Go services.", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ViaRecruiter(tt.desc); got != tt.want {
				t.Fatalf("ViaRecruiter(%q) = %v, want %v", tt.desc, got, tt.want)
			}
		})
	}
}
