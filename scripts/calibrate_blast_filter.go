package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

type Row struct {
	File      string
	Label     string
	Protein   string
	GeneKey   string
	Identity  float64
	Coverage  float64
	EValue    float64
	Ratio     float64
	InterPro  string
	Reviewed  string
	Accession string
	Fragment  string
	Caution   string
}

type Config struct {
	Name                string
	MinIdentity         float64
	MinCoverage         float64
	MaxEValue           float64
	RequireRatio        bool
	MinRatio            float64
	MaxRatio            float64
	RequireInterPro     bool
	AllowPartial        bool
	RejectMissing       bool
	RejectUncertain     bool
	RejectBlankInterPro bool
	MinInterProCoverage float64
	KeepBestIsoform     bool
}

type Result struct {
	Config       Config
	Total        int
	Kept         int
	ByFolder     map[string]int
	ByFile       map[string]int
	ByStatusKept map[string]int
}

func main() {
	root := "bin/output"
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	rows, err := readRows(root)
	if err != nil {
		panic(err)
	}
	fmt.Printf("rows=%d files=%d\n", len(rows), countFiles(rows))
	fmt.Println("current/default-like and grid results:")

	configs := []Config{
		{Name: "current-default", MinIdentity: 0, MinCoverage: 0, MaxEValue: 0, RequireRatio: true, MinRatio: 70, MaxRatio: 130, RequireInterPro: true, AllowPartial: true, RejectMissing: true, RejectUncertain: true, RejectBlankInterPro: true, KeepBestIsoform: true},
		{Name: "strict-blast-reference", MinIdentity: 30, MinCoverage: 50, MaxEValue: 1e-30, RequireRatio: true, MinRatio: 70, MaxRatio: 130, RequireInterPro: true, AllowPartial: true, RejectMissing: true, RejectUncertain: true, RejectBlankInterPro: true, KeepBestIsoform: true},
	}
	for _, id := range []float64{25, 30, 35, 40, 45, 50} {
		for _, cov := range []float64{40, 50, 60, 70} {
			for _, eval := range []float64{1e-10, 1e-20, 1e-30, 1e-40, 1e-50} {
				configs = append(configs, Config{
					Name:                fmt.Sprintf("id%.0f_cov%.0f_e%.0g_ratio70-130_ip", id, cov, eval),
					MinIdentity:         id,
					MinCoverage:         cov,
					MaxEValue:           eval,
					RequireRatio:        true,
					MinRatio:            70,
					MaxRatio:            130,
					RequireInterPro:     true,
					AllowPartial:        true,
					RejectMissing:       true,
					RejectUncertain:     true,
					RejectBlankInterPro: true,
					KeepBestIsoform:     true,
				})
			}
		}
	}
	for _, rmin := range []float64{60, 65, 70, 75, 80} {
		for _, rmax := range []float64{120, 130, 140, 150} {
			configs = append(configs, Config{
				Name:                fmt.Sprintf("id30_cov50_e1e-30_ratio%.0f-%.0f_ip", rmin, rmax),
				MinIdentity:         30,
				MinCoverage:         50,
				MaxEValue:           1e-30,
				RequireRatio:        true,
				MinRatio:            rmin,
				MaxRatio:            rmax,
				RequireInterPro:     true,
				AllowPartial:        true,
				RejectMissing:       true,
				RejectUncertain:     true,
				RejectBlankInterPro: true,
				KeepBestIsoform:     true,
			})
		}
	}

	results := make([]Result, 0, len(configs))
	for _, cfg := range configs {
		results = append(results, apply(rows, cfg))
	}
	sort.Slice(results, func(i, j int) bool {
		di := math.Abs(float64(results[i].Kept - 63))
		dj := math.Abs(float64(results[j].Kept - 63))
		if di != dj {
			return di < dj
		}
		return results[i].Kept < results[j].Kept
	})
	for i, res := range results {
		if i >= 30 {
			break
		}
		fmt.Printf("%-42s kept=%4d/%4d diff63=%4.0f statuses=%v\n", res.Config.Name, res.Kept, res.Total, math.Abs(float64(res.Kept-63)), res.ByStatusKept)
	}
	fmt.Println("\ncurrent-default by file:")
	current := apply(rows, configs[0])
	keys := make([]string, 0, len(current.ByFile))
	for k := range current.ByFile {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("%-95s %d\n", k, current.ByFile[k])
	}
}

func readRows(root string) ([]Row, error) {
	var rows []Row
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".xlsx") {
			return nil
		}
		fileRows, err := readXLSX(path, root)
		if err != nil {
			return err
		}
		rows = append(rows, fileRows...)
		return nil
	})
	return rows, err
}

func readXLSX(path, root string) ([]Row, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sheet := f.GetSheetName(0)
	raw, err := f.GetRows(sheet)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	headers := map[string]int{}
	for i, h := range raw[0] {
		headers[strings.ToLower(strings.TrimSpace(h))] = i
	}
	rel, _ := filepath.Rel(root, path)
	out := make([]Row, 0, len(raw)-1)
	for _, r := range raw[1:] {
		get := func(names ...string) string {
			for _, name := range names {
				if idx, ok := headers[strings.ToLower(name)]; ok && idx < len(r) {
					if v := strings.TrimSpace(r[idx]); v != "" {
						return v
					}
				}
			}
			return ""
		}
		label := get("label_name")
		if label == "" {
			label = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}
		protein := get("protein", "subject_id")
		row := Row{
			File:      rel,
			Label:     label,
			Protein:   protein,
			GeneKey:   geneKey(protein),
			Identity:  parseFloat(get("identity (%)")),
			Coverage:  parseFloat(get("align_len / query_length (%)")),
			EValue:    parseEValue(get("e_value")),
			Ratio:     parseFloat(get("lemna target_length / UniProt canonical length (%)", "Phytozome target_length / UniProt canonical length (%)", "target_length / UniProt canonical length (%)")),
			InterPro:  strings.ToLower(get("InterPro conserved region status")),
			Reviewed:  strings.ToLower(get("UniProt reviewed")),
			Accession: get("UniProt accession"),
			Fragment:  strings.ToLower(get("UniProt fragment")),
			Caution:   get("UniProt sequence caution"),
		}
		out = append(out, row)
	}
	return out, nil
}

func apply(rows []Row, cfg Config) Result {
	selected := make([]bool, len(rows))
	for i, row := range rows {
		selected[i] = keep(row, cfg)
	}
	if cfg.KeepBestIsoform {
		best := map[string]int{}
		for i, row := range rows {
			if !selected[i] || row.GeneKey == "" {
				continue
			}
			key := strings.ToLower(filepath.Dir(row.File)) + "\x00" + row.GeneKey
			if old, ok := best[key]; !ok || better(row, rows[old]) {
				if ok {
					selected[old] = false
				}
				best[key] = i
			} else {
				selected[i] = false
			}
		}
	}
	res := Result{Config: cfg, Total: len(rows), ByFolder: map[string]int{}, ByFile: map[string]int{}, ByStatusKept: map[string]int{}}
	for i, ok := range selected {
		if !ok {
			continue
		}
		res.Kept++
		folder := strings.Split(rows[i].File, string(filepath.Separator))[0]
		res.ByFolder[folder]++
		res.ByFile[rows[i].File]++
		status := rows[i].InterPro
		if status == "" {
			status = "<blank>"
		}
		res.ByStatusKept[status]++
	}
	return res
}

func keep(row Row, cfg Config) bool {
	if row.Identity < cfg.MinIdentity {
		return false
	}
	if row.Coverage < cfg.MinCoverage {
		return false
	}
	if cfg.MaxEValue > 0 && row.EValue > cfg.MaxEValue {
		return false
	}
	if cfg.RequireRatio && row.Ratio <= 0 {
		return false
	}
	if row.Ratio > 0 && (row.Ratio < cfg.MinRatio || row.Ratio > cfg.MaxRatio) {
		return false
	}
	if cfg.RequireInterPro {
		switch row.InterPro {
		case "present":
		case "partial":
			if !cfg.AllowPartial {
				return false
			}
		default:
			return false
		}
	} else {
		if cfg.RejectMissing && row.InterPro == "missing" {
			return false
		}
		if cfg.RejectUncertain && row.InterPro == "uncertain" {
			return false
		}
		if cfg.RejectBlankInterPro && row.InterPro == "" {
			return false
		}
	}
	if isTruthy(row.Fragment) {
		return false
	}
	return true
}

func better(a, b Row) bool {
	as, bs := evidenceScore(a), evidenceScore(b)
	if as != bs {
		return as > bs
	}
	if a.Identity != b.Identity {
		return a.Identity > b.Identity
	}
	if a.Coverage != b.Coverage {
		return a.Coverage > b.Coverage
	}
	if a.EValue != b.EValue {
		return a.EValue < b.EValue
	}
	return false
}

func evidenceScore(row Row) int {
	score := 0
	switch row.InterPro {
	case "present":
		score += 80
	case "partial":
		score += 35
	}
	if row.Ratio > 0 {
		dist := math.Abs(row.Ratio - 100)
		if dist <= 10 {
			score += 20
		} else if dist <= 30 {
			score += 8
		}
	}
	if row.Accession != "" {
		score += 25
	}
	if row.Reviewed == "reviewed" {
		score += 25
	}
	return score
}

func geneKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimSuffix(value, "/")
	if idx := strings.LastIndex(value, "/"); idx >= 0 {
		value = value[idx+1:]
	}
	value = regexp.MustCompile(`(?i)_t\d+$`).ReplaceAllString(value, "")
	value = regexp.MustCompile(`(?i)[._-]t\d+$`).ReplaceAllString(value, "")
	value = regexp.MustCompile(`(?i)\.\d+$`).ReplaceAllString(value, "")
	return value
}

func parseFloat(value string) float64 {
	value = strings.TrimSpace(strings.TrimSuffix(value, "%"))
	if value == "" {
		return 0
	}
	re := regexp.MustCompile(`[-+]?\d*\.?\d+(?:[eE][-+]?\d+)?`)
	m := re.FindString(value)
	if m == "" {
		return 0
	}
	v, _ := strconv.ParseFloat(m, 64)
	return v
}

func parseEValue(value string) float64 {
	if strings.TrimSpace(value) == "" {
		return math.Inf(1)
	}
	v := parseFloat(value)
	return v
}

func isTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "false", "no", "0", "none", "not fragment":
		return false
	default:
		return true
	}
}

func countFiles(rows []Row) int {
	seen := map[string]bool{}
	for _, row := range rows {
		seen[row.File] = true
	}
	return len(seen)
}
