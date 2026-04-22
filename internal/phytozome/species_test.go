package phytozome

import "testing"

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

	candidates := parseSpeciesCandidates(html)
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
}

func TestFilterSpeciesCandidates(t *testing.T) {
	candidates := parseSpeciesCandidates(`
<tr><td><a href="/info/Taestivumcv_ChineseSpring_v2_1">Triticum aestivum cv. Chinese Spring v2.1</a></td><td>bread wheat</td><td>Sep 29, 2022</td></tr>
<tr><td><a href="/info/Spolyrhiza_v2">S. polyrhiza v2</a></td><td>greater duckweed</td><td>Jan 1, 2020</td></tr>`)

	filtered := FilterSpeciesCandidates(candidates, "wheat")
	if len(filtered) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(filtered))
	}

	if filtered[0].JBrowseName != "Taestivumcv_ChineseSpring_v2_1" {
		t.Fatalf("unexpected filtered jbrowse name: %q", filtered[0].JBrowseName)
	}
}
