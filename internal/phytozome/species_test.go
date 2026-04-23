package phytozome

import (
	"reflect"
	"testing"
)

func TestParseSpeciesCandidates(t *testing.T) {
	html := `
<table>
  <tbody>
    <tr>
      <td><a href="/info/Taestivumcv_ChineseSpring_v2_1">Triticum aestivum cv. Chinese Spring v2.1</a></td>
      <td>bread wheat</td>
      <td>Sep 29, 2022</td>
    </tr>
    <tr>
      <td><a href="/info/Spolyrhiza_v2">S. polyrhiza v2</a></td>
      <td>greater duckweed</td>
      <td>Jan 1, 2020</td>
    </tr>
  </tbody>
</table>`

	targets := map[string]targetRecord{
		"Taestivumcv_ChineseSpring_v2_1": {
			ProteomeID: 725,
			Attributes: struct {
				CommonName     string `json:"commonName"`
				DisplayName    string `json:"displayName"`
				DisplayVersion string `json:"displayVersion"`
				JBrowseName    string `json:"jbrowseName"`
			}{
				CommonName:     "bread wheat",
				DisplayName:    "Triticum aestivum cv. Chinese Spring",
				DisplayVersion: "v2.1",
				JBrowseName:    "Taestivumcv_ChineseSpring_v2_1",
			},
		},
		"Spolyrhiza_v2": {
			ProteomeID: 123,
			Attributes: struct {
				CommonName     string `json:"commonName"`
				DisplayName    string `json:"displayName"`
				DisplayVersion string `json:"displayVersion"`
				JBrowseName    string `json:"jbrowseName"`
			}{
				CommonName:     "greater duckweed",
				DisplayName:    "Spirodela polyrhiza",
				DisplayVersion: "v2",
				JBrowseName:    "Spolyrhiza_v2",
			},
		},
	}

	candidates := parseSpeciesCandidates(html, targets)
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}

	if candidates[0].JBrowseName != "Spolyrhiza_v2" {
		t.Fatalf("expected alphabetical sort to place Spolyrhiza_v2 first, got %q", candidates[0].JBrowseName)
	}

	if candidates[1].JBrowseName != "Taestivumcv_ChineseSpring_v2_1" {
		t.Fatalf("unexpected second jbrowse name: %q", candidates[1].JBrowseName)
	}

	if candidates[1].CommonName != "bread wheat" {
		t.Fatalf("unexpected common name: %q", candidates[1].CommonName)
	}

	if candidates[1].ProteomeID != 725 {
		t.Fatalf("unexpected proteome id: %d", candidates[1].ProteomeID)
	}

	if candidates[0].SearchAlias != "Spirodela polyrhiza v2" {
		t.Fatalf("unexpected search alias: %q", candidates[0].SearchAlias)
	}
}

func TestFilterSpeciesCandidates(t *testing.T) {
	targets := map[string]targetRecord{
		"Taestivumcv_ChineseSpring_v2_1": {
			ProteomeID: 725,
			Attributes: struct {
				CommonName     string `json:"commonName"`
				DisplayName    string `json:"displayName"`
				DisplayVersion string `json:"displayVersion"`
				JBrowseName    string `json:"jbrowseName"`
			}{
				CommonName:     "bread wheat",
				DisplayName:    "Triticum aestivum cv. Chinese Spring",
				DisplayVersion: "v2.1",
				JBrowseName:    "Taestivumcv_ChineseSpring_v2_1",
			},
		},
		"Spolyrhiza_v2": {
			ProteomeID: 123,
			Attributes: struct {
				CommonName     string `json:"commonName"`
				DisplayName    string `json:"displayName"`
				DisplayVersion string `json:"displayVersion"`
				JBrowseName    string `json:"jbrowseName"`
			}{
				CommonName:     "greater duckweed",
				DisplayName:    "Spirodela polyrhiza",
				DisplayVersion: "v2",
				JBrowseName:    "Spolyrhiza_v2",
			},
		},
	}

	candidates := parseSpeciesCandidates(`
<tr><td><a href="/info/Taestivumcv_ChineseSpring_v2_1">Triticum aestivum cv. Chinese Spring v2.1</a></td><td>bread wheat</td><td>Sep 29, 2022</td></tr>
<tr><td><a href="/info/Spolyrhiza_v2">S. polyrhiza v2</a></td><td>greater duckweed</td><td>Jan 1, 2020</td></tr>`, targets)

	filtered := FilterSpeciesCandidates(candidates, "wheat")
	if len(filtered) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(filtered))
	}

	if filtered[0].JBrowseName != "Taestivumcv_ChineseSpring_v2_1" {
		t.Fatalf("unexpected filtered jbrowse name: %q", filtered[0].JBrowseName)
	}

	filtered = FilterSpeciesCandidates(candidates, "spirodela")
	if len(filtered) != 1 {
		t.Fatalf("expected 1 candidate for spirodela, got %d", len(filtered))
	}

	if filtered[0].JBrowseName != "Spolyrhiza_v2" {
		t.Fatalf("unexpected filtered jbrowse name for spirodela: %q", filtered[0].JBrowseName)
	}

	filtered = FilterSpeciesCandidates(candidates, "spirodela polyrhiza v2")
	if len(filtered) != 1 {
		t.Fatalf("expected 1 candidate for full spirodela name, got %d", len(filtered))
	}

	filtered = FilterSpeciesCandidates(candidates, "spiro")
	if len(filtered) != 1 {
		t.Fatalf("expected 1 candidate for spiro prefix, got %d", len(filtered))
	}
}

func TestExtractBundleScriptURLs(t *testing.T) {
	homePage := []byte(`
<html><body>
<script src="/vendors-abc.js"></script>
<script src="/main-abc.js"></script>
<script src="/archaeopteryx-abc.js"></script>
</body></html>`)

	got, err := extractBundleScriptURLs(homePage)
	if err != nil {
		t.Fatalf("extractBundleScriptURLs returned error: %v", err)
	}

	want := []string{
		"https://phytozome-next.jgi.doe.gov/main-abc.js",
		"https://phytozome-next.jgi.doe.gov/archaeopteryx-abc.js",
		"https://phytozome-next.jgi.doe.gov/vendors-abc.js",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected script urls: got %v want %v", got, want)
	}
}

func TestExtractTargetRecordsFallsBackToJSONObjectScan(t *testing.T) {
	bundle := []byte(`window.x={"name":"Arabidopsis thaliana","taxId":"3702","proteomeId":167,"attributes":{"jbrowseName":"Athaliana_TAIR10","displayName":"Arabidopsis thaliana","displayVersion":"TAIR10","commonName":"thale cress"}};`)

	targets, err := extractTargetRecords(bundle)
	if err != nil {
		t.Fatalf("extractTargetRecords returned error: %v", err)
	}

	target, ok := targets["Athaliana_TAIR10"]
	if !ok {
		t.Fatalf("expected Athaliana_TAIR10 in targets: %v", targets)
	}
	if target.ProteomeID != 167 {
		t.Fatalf("unexpected proteome id: %d", target.ProteomeID)
	}
	if target.Attributes.DisplayVersion != "TAIR10" {
		t.Fatalf("unexpected display version: %q", target.Attributes.DisplayVersion)
	}
}

func TestFindMatchingJSONObjectEndHandlesBracesInStrings(t *testing.T) {
	value := []byte(`{"a":"{x}","b":{"c":"d"}} trailing`)
	end, ok := findMatchingJSONObjectEnd(value, 0)
	if !ok {
		t.Fatalf("expected to find matching object end")
	}
	if got := string(value[:end+1]); got != `{"a":"{x}","b":{"c":"d"}}` {
		t.Fatalf("unexpected object slice: %q", got)
	}
}

func TestCandidatesFromTargetsIncludeNonOverviewSpecies(t *testing.T) {
	targets := map[string]targetRecord{
		"Spolyrhiza_v2": {
			ProteomeID: 290,
			Name:       "Spirodela polyrhiza",
			Attributes: struct {
				CommonName     string `json:"commonName"`
				DisplayName    string `json:"displayName"`
				DisplayVersion string `json:"displayVersion"`
				JBrowseName    string `json:"jbrowseName"`
			}{
				CommonName:     "greater duckweed",
				DisplayName:    "Spirodela polyrhiza",
				DisplayVersion: "v2",
				JBrowseName:    "Spolyrhiza_v2",
			},
		},
	}

	candidates := candidatesFromTargets(targets, map[string]string{})
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}

	if candidates[0].JBrowseName != "Spolyrhiza_v2" {
		t.Fatalf("unexpected jbrowse name: %q", candidates[0].JBrowseName)
	}

	if candidates[0].DisplayLabel() != "Spirodela polyrhiza v2 (greater duckweed)" {
		t.Fatalf("unexpected display label: %q", candidates[0].DisplayLabel())
	}
}

func TestBlastResultsPending(t *testing.T) {
	if !blastResultsPending(500, "Couldn't find those results, or they're incomplete.") {
		t.Fatalf("expected incomplete 500 to be treated as pending")
	}
	if !blastResultsPending(202, "pending") {
		t.Fatalf("expected 202 to be treated as pending")
	}
	if blastResultsPending(500, "server exploded") {
		t.Fatalf("unexpected generic 500 treated as pending")
	}
}
