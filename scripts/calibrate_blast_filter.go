// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

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
	File             string
	Folder           string
	Base             string
	Label            string
	Protein          string
	GeneKey          string
	Identity         float64
	Coverage         float64
	EValue           float64
	Ratio            float64
	InterPro         string
	InterProCoverage float64
	Reviewed         string
	Accession        string
	Fragment         string
	Caution          string
}

type Config struct {
	Name                            string
	MinIdentity                     float64
	MinCoverage                     float64
	MaxEValue                       float64
	RequireRatio                    bool
	MinRatio                        float64
	MaxRatio                        float64
	RequireInterPro                 bool
	AllowPartial                    bool
	RejectMissing                   bool
	RejectUncertain                 bool
	RejectBlankInterPro             bool
	MinInterProCoverage             float64
	RequireInterProCoverageWhenUsed bool
	KeepBestIsoform                 bool
}

type TargetSet struct {
	Name     string
	Folder   string
	ByFamily map[string]int
}

type Result struct {
	Config          Config
	Total           int
	Kept            int
	ByFolder        map[string]int
	ByFamily        map[string]int
	ByStatusKept    map[string]int
	TargetSummaries []TargetSummary
	Score           float64
}

type TargetSummary struct {
	Name          string
	Folder        string
	ExpectedTotal int
	ObservedTotal int
	TotalAbsDiff  int
	FamilyAbsDiff int
	FamilyDetails []FamilyDiff
}

type FamilyDiff struct {
	Family   string
	Expected int
	Observed int
	AbsDiff  int
}

func main() {
	root := `C:\Users\wangsychn\Desktop\phytozome-go_v20260505T202930Z_windows_amd64\output`
	if len(os.Args) > 1 && strings.TrimSpace(os.Args[1]) != "" {
		root = os.Args[1]
	}
	rows, err := readRows(root)
	if err != nil {
		panic(err)
	}

	targets := []TargetSet{
		{
			Name:   "Table17 Monolignol Biosynthesis",
			Folder: "Monolignol_Biosynthesis",
			ByFamily: map[string]int{
				"CAD":    4,
				"CCOAMT": 1,
				"4CL":    9,
				"CCR":    21,
				"PAL":    3,
				"C4H":    3,
				"HCT":    20,
				"COMT":   5,
				"C3H":    1,
				"F5H":    3,
			},
		},
		{
			Name:   "Table16 Cellulose",
			Folder: "Cellulose",
			ByFamily: map[string]int{
				"CESA": 10,
			},
		},
		{
			Name:   "Table16 Hemicelluloses",
			Folder: "Hemicelluloses",
			ByFamily: map[string]int{
				"IRX": 21,
			},
		},
	}

	fmt.Printf("rows=%d files=%d folders=%d\n", len(rows), countFiles(rows), countFolders(rows))

	configs := candidateConfigs()
	results := make([]Result, 0, len(configs))
	for _, cfg := range configs {
		results = append(results, apply(rows, cfg, targets))
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score < results[j].Score
		}
		if results[i].Kept != results[j].Kept {
			return results[i].Kept < results[j].Kept
		}
		return results[i].Config.Name < results[j].Config.Name
	})

	fmt.Println("\nTop calibration results:")
	for i, res := range results {
		if i >= 20 {
			break
		}
		fmt.Printf("%-44s score=%6.1f kept=%4d", res.Config.Name, res.Score, res.Kept)
		for _, summary := range res.TargetSummaries {
			fmt.Printf(" | %s total %d/%d diff=%d famdiff=%d", shortName(summary.Folder), summary.ObservedTotal, summary.ExpectedTotal, summary.TotalAbsDiff, summary.FamilyAbsDiff)
		}
		fmt.Println()
	}

	if len(results) == 0 {
		return
	}
	best := results[0]
	fmt.Printf("\nBest config: %s\n", best.Config.Name)
	fmt.Printf("Settings: minIdentity=%.1f minCoverage=%.1f maxEValue=%g ratio=%t %.1f-%.1f interProReq=%t allowPartial=%t interProCov=%.1f requireInterProCov=%t keepBestIsoform=%t\n",
		best.Config.MinIdentity, best.Config.MinCoverage, best.Config.MaxEValue, best.Config.RequireRatio, best.Config.MinRatio, best.Config.MaxRatio, best.Config.RequireInterPro, best.Config.AllowPartial, best.Config.MinInterProCoverage, best.Config.RequireInterProCoverageWhenUsed, best.Config.KeepBestIsoform)
	for _, summary := range best.TargetSummaries {
		fmt.Printf("\n[%s] observed=%d expected=%d totalDiff=%d familyDiff=%d\n", summary.Name, summary.ObservedTotal, summary.ExpectedTotal, summary.TotalAbsDiff, summary.FamilyAbsDiff)
		for _, diff := range summary.FamilyDetails {
			fmt.Printf("  %-8s observed=%3d expected=%3d diff=%3d\n", diff.Family, diff.Observed, diff.Expected, diff.AbsDiff)
		}
	}
}

func candidateConfigs() []Config {
	configs := []Config{
		{
			Name:                "current-default-like",
			RequireRatio:        true,
			MinRatio:            70,
			MaxRatio:            130,
			RequireInterPro:     true,
			AllowPartial:        true,
			RejectMissing:       true,
			RejectUncertain:     true,
			RejectBlankInterPro: true,
			KeepBestIsoform:     true,
		},
		{
			Name:                "paper-tight-30-50-1e30",
			MinIdentity:         30,
			MinCoverage:         50,
			MaxEValue:           1e-30,
			RequireRatio:        true,
			MinRatio:            70,
			MaxRatio:            130,
			RequireInterPro:     true,
			AllowPartial:        true,
			RejectMissing:       true,
			RejectUncertain:     true,
			RejectBlankInterPro: true,
			KeepBestIsoform:     true,
		},
	}
	for _, id := range []float64{0, 25, 30, 35, 40} {
		for _, cov := range []float64{0, 40, 50, 60} {
			for _, eval := range []float64{0, 1e-20, 1e-30, 1e-40} {
				for _, rmin := range []float64{65, 70, 75} {
					for _, rmax := range []float64{125, 130, 140} {
						for _, ipCov := range []float64{0, 20, 40} {
							for _, allowPartial := range []bool{true, false} {
								configs = append(configs, Config{
									Name:                            fmt.Sprintf("id%.0f_cov%.0f_e%.0g_r%.0f-%.0f_ipcov%.0f_partial%t", id, cov, eval, rmin, rmax, ipCov, allowPartial),
									MinIdentity:                     id,
									MinCoverage:                     cov,
									MaxEValue:                       eval,
									RequireRatio:                    true,
									MinRatio:                        rmin,
									MaxRatio:                        rmax,
									RequireInterPro:                 true,
									AllowPartial:                    allowPartial,
									RejectMissing:                   true,
									RejectUncertain:                 true,
									RejectBlankInterPro:             true,
									MinInterProCoverage:             ipCov,
									RequireInterProCoverageWhenUsed: ipCov > 0,
									KeepBestIsoform:                 true,
								})
							}
						}
					}
				}
			}
		}
	}
	return configs
}

func readRows(root string) ([]Row, error) {
	var rows []Row
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".xlsx") || strings.HasSuffix(strings.ToLower(path), "_raw.xlsx") {
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
	sheet := "BLAST Results"
	if idx, err := f.GetSheetIndex(sheet); err != nil || idx == -1 {
		sheet = f.GetSheetName(0)
	}
	raw, err := f.GetRows(sheet)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	headers := map[string]int{}
	for i, h := range raw[0] {
		headers[normalizeHeader(h)] = i
	}
	rel, _ := filepath.Rel(root, path)
	folder := filepath.Base(filepath.Dir(path))
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	out := make([]Row, 0, len(raw)-1)
	for _, r := range raw[1:] {
		get := func(names ...string) string {
			for _, name := range names {
				if idx, ok := headers[normalizeHeader(name)]; ok && idx < len(r) {
					if v := strings.TrimSpace(r[idx]); v != "" {
						return v
					}
				}
			}
			return ""
		}
		protein := get("protein", "subject_id")
		if protein == "" {
			continue
		}
		label := get("label_name")
		if label == "" {
			label = base
		}
		out = append(out, Row{
			File:             rel,
			Folder:           folder,
			Base:             base,
			Label:            label,
			Protein:          protein,
			GeneKey:          geneKey(protein),
			Identity:         parseFloat(get("percent_identity", "identity (%)")),
			Coverage:         parseFloat(get("align_query_length_percent", "align_len / query_length (%)")),
			EValue:           parseEValue(get("e_value")),
			Ratio:            parseFloat(get("lemna target_length / UniProt canonical length (%)", "Phytozome target_length / UniProt canonical length (%)", "target_length / UniProt canonical length (%)")),
			InterPro:         strings.ToLower(get("InterPro conserved region status")),
			InterProCoverage: parseFloat(get("InterPro coverage (%)", "InterPro coverage percent", "InterPro coverage")),
			Reviewed:         strings.ToLower(get("UniProt reviewed")),
			Accession:        get("UniProt accession"),
			Fragment:         strings.ToLower(get("UniProt fragment")),
			Caution:          get("UniProt sequence caution"),
		})
	}
	return out, nil
}

func apply(rows []Row, cfg Config, targets []TargetSet) Result {
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
			key := strings.ToLower(row.Folder) + "\x00" + row.GeneKey
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
	res := Result{
		Config:       cfg,
		Total:        len(rows),
		ByFolder:     map[string]int{},
		ByFamily:     map[string]int{},
		ByStatusKept: map[string]int{},
	}
	for i, ok := range selected {
		if !ok {
			continue
		}
		res.Kept++
		row := rows[i]
		res.ByFolder[row.Folder]++
		family := canonicalFamily(row.Base)
		res.ByFamily[row.Folder+"\x00"+family]++
		status := row.InterPro
		if status == "" {
			status = "<blank>"
		}
		res.ByStatusKept[status]++
	}
	res.TargetSummaries = summarizeTargets(res.ByFamily, res.ByFolder, targets)
	for _, summary := range res.TargetSummaries {
		res.Score += float64(summary.TotalAbsDiff*4 + summary.FamilyAbsDiff)
	}
	return res
}

func summarizeTargets(byFamily map[string]int, byFolder map[string]int, targets []TargetSet) []TargetSummary {
	out := make([]TargetSummary, 0, len(targets))
	for _, target := range targets {
		summary := TargetSummary{
			Name:          target.Name,
			Folder:        target.Folder,
			ExpectedTotal: sumFamilyMap(target.ByFamily),
			ObservedTotal: byFolder[target.Folder],
		}
		summary.TotalAbsDiff = absInt(summary.ObservedTotal - summary.ExpectedTotal)
		families := make([]string, 0, len(target.ByFamily))
		for family := range target.ByFamily {
			families = append(families, family)
		}
		sort.Strings(families)
		for _, family := range families {
			observed := byFamily[target.Folder+"\x00"+family]
			diff := FamilyDiff{
				Family:   family,
				Expected: target.ByFamily[family],
				Observed: observed,
				AbsDiff:  absInt(observed - target.ByFamily[family]),
			}
			summary.FamilyAbsDiff += diff.AbsDiff
			summary.FamilyDetails = append(summary.FamilyDetails, diff)
		}
		out = append(out, summary)
	}
	return out
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
	if cfg.MinInterProCoverage > 0 {
		if row.InterProCoverage <= 0 && cfg.RequireInterProCoverageWhenUsed {
			return false
		}
		if row.InterProCoverage > 0 && row.InterProCoverage < cfg.MinInterProCoverage {
			return false
		}
	}
	if cfg.RequireInterPro {
		switch row.InterPro {
		case "present":
		case "partial":
			if !cfg.AllowPartial {
				return false
			}
		case "uncertain":
			if cfg.RejectUncertain {
				return false
			}
		case "missing":
			if cfg.RejectMissing {
				return false
			}
		case "":
			if cfg.RejectBlankInterPro {
				return false
			}
		default:
			return false
		}
	}
	if isTruthy(row.Fragment) {
		return false
	}
	if strings.TrimSpace(row.Caution) != "" {
		return false
	}
	return true
}

func better(a, b Row) bool {
	as, bs := evidenceScore(a), evidenceScore(b)
	if as != bs {
		return as > bs
	}
	if a.EValue != b.EValue {
		return a.EValue < b.EValue
	}
	if a.Identity != b.Identity {
		return a.Identity > b.Identity
	}
	if a.Coverage != b.Coverage {
		return a.Coverage > b.Coverage
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
		switch {
		case dist <= 10:
			score += 20
		case dist <= 30:
			score += 10
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

func canonicalFamily(name string) string {
	s := strings.TrimSpace(strings.ToUpper(name))
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "'", "")
	s = strings.ReplaceAll(s, "_", "")
	switch {
	case strings.HasPrefix(s, "CAD"):
		return "CAD"
	case strings.HasPrefix(s, "CCOA") || strings.HasPrefix(s, "CCOAMT"):
		return "CCOAMT"
	case strings.HasPrefix(s, "4CL"):
		return "4CL"
	case strings.HasPrefix(s, "CCR"):
		return "CCR"
	case strings.HasPrefix(s, "PAL"):
		return "PAL"
	case strings.HasPrefix(s, "C4H"):
		return "C4H"
	case strings.HasPrefix(s, "HCT"):
		return "HCT"
	case strings.HasPrefix(s, "COMT"):
		return "COMT"
	case strings.HasPrefix(s, "C3H"):
		return "C3H"
	case strings.HasPrefix(s, "F5H"):
		return "F5H"
	case strings.HasPrefix(s, "IRX"):
		return "IRX"
	case strings.HasPrefix(s, "CESA"):
		return "CESA"
	default:
		return s
	}
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

func normalizeHeader(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.ReplaceAll(value, "  ", " ")
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
	return parseFloat(value)
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

func countFolders(rows []Row) int {
	seen := map[string]bool{}
	for _, row := range rows {
		seen[row.Folder] = true
	}
	return len(seen)
}

func sumFamilyMap(values map[string]int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func shortName(value string) string {
	return strings.ReplaceAll(value, "_", "")
}
