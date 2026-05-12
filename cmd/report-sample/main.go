// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/KiriKirby/phytozome-go/internal/report"
)

func main() {
	outDir := flag.String("out", filepath.Join("docs", "reports", "samples"), "directory for the sample PDF")
	name := flag.String("name", "", "optional output file name")
	mode := flag.String("mode", "keyword", "sample report mode: keyword or blast")
	scenario := flag.String("scenario", "", "optional sample scenario: keyword-lemna, keyword-phytozome, blast-lemna, blast-phytozome, blast-phytozome-no-refs")
	flag.Parse()

	var data report.ReportData
	var render func(string, report.ReportData) error
	switch scenarioKey(*mode, *scenario) {
	case "keyword", "keyword-lemna":
		data = report.SampleKeywordReportData()
		render = report.RenderKeywordPDF
	case "keyword-phytozome":
		data = report.SampleKeywordPhytozomeReportData()
		render = report.RenderKeywordPDF
	case "blast", "blast-lemna":
		data = report.SampleBlastReportData()
		render = report.RenderBlastPDF
	case "blast-phytozome":
		data = report.SampleBlastPhytozomeReportData()
		render = report.RenderBlastPDF
	case "blast-phytozome-no-refs":
		data = report.SampleBlastWithoutReferencesReportData()
		render = report.RenderBlastPDF
	default:
		fmt.Fprintf(os.Stderr, "unsupported scenario %q for mode %q\n", *scenario, *mode)
		os.Exit(2)
	}

	fileName := *name
	if fileName == "" {
		baseName := sampleReportBaseName(data)
		if baseName == "" {
			baseName = *mode
		}
		fileName = report.ReportFileNameForBase(baseName, data.GeneratedAt)
	}
	path := filepath.Join(*outDir, fileName)
	if err := render(path, data); err != nil {
		fmt.Fprintf(os.Stderr, "render sample report: %v\n", err)
		os.Exit(1)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	fmt.Println(abs)
}

func scenarioKey(mode string, scenario string) string {
	scenario = filepath.Clean(scenario)
	if scenario == "." || scenario == "" {
		return mode
	}
	return scenario
}

func sampleReportBaseName(data report.ReportData) string {
	values := data.Keyword.ExportSettings
	if data.Mode == "blast" {
		values = data.Blast.ExportSettings
	}
	for _, value := range values {
		if value.Name == "File base name" && value.Value != "" {
			return value.Value
		}
	}
	return ""
}
