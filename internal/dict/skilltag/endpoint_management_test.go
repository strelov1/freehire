package skilltag

import (
	"reflect"
	"testing"
)

// TestParse_EndpointManagement covers the device-management tools endpoint engineers are
// hired for, and the bare words that must not reach them.
func TestParse_EndpointManagement(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"mdm stack", "Manage Macs in Kandji and Windows with Intune, Windows Autopilot and Tanium.",
			[]string{"intune", "kandji", "tanium", "windows-autopilot"}},
		{"rmm", "Patch servers through NinjaOne.", []string{"ninjaone"}},
		{"workspace one spellings", "Migrate VMware Workspace ONE (AirWatch) to Omnissa Workspace ONE UEM.",
			[]string{"vmware", "workspace-one"}},
		{"defender for endpoint", "Onboard devices to Defender for Endpoint.", []string{"microsoft-defender"}},

		{"bare autopilot is not windows", "Train neural networks for Autopilot in Python.", []string{"neural-networks", "python"}},
		{"bare workspace one is not the product", "Hybrid: a shared workspace one day a week.", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Parse(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Parse(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}
