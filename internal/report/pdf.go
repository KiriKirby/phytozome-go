// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package report

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/phpdave11/gofpdf"
)

const (
	pageWidth    = 595.28
	pageHeight   = 841.89
	marginLeft   = 44.0
	marginRight  = 44.0
	marginTop    = 54.0
	marginBottom = 54.0
)

const (
	reportSansFamily = "ReportSystemSans"
	fontRegular      = "F1"
	fontBold         = "F2"
	fontMono         = "F3"
)

type pdfColor struct {
	R float64
	G float64
	B float64
}

var (
	colorText      = pdfColor{0.12, 0.14, 0.16}
	colorMuted     = pdfColor{0.38, 0.42, 0.46}
	colorRule      = pdfColor{0.78, 0.81, 0.84}
	colorFill      = pdfColor{0.96, 0.97, 0.98}
	colorHeader    = pdfColor{0.91, 0.94, 0.96}
	colorPrimary   = pdfColor{0.05, 0.38, 0.52}
	colorSecondary = pdfColor{0.43, 0.47, 0.52}
	colorSuccess   = pdfColor{0.10, 0.47, 0.28}
	colorWarning   = pdfColor{0.76, 0.48, 0.05}
	colorError     = pdfColor{0.70, 0.18, 0.16}
	colorPurple    = pdfColor{0.38, 0.29, 0.62}
	colorMissing   = pdfColor{0.74, 0.76, 0.78}
)

type pdfDocument struct {
	title       string
	generatedAt time.Time
	pdf         *gofpdf.Fpdf
	fontsLoaded bool
	fontErr     error
	fontInfo    systemFontInfo
}

type pdfReportRenderer struct {
	doc     *pdfDocument
	y       float64
	chapter string
}

func newPDFReport(title string, generatedAt time.Time) *pdfReportRenderer {
	if generatedAt.IsZero() {
		generatedAt = time.Now()
	}
	if strings.TrimSpace(title) == "" {
		title = "Data Analysis Report"
	}

	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.SetMargins(marginLeft, marginTop, marginRight)
	pdf.SetAutoPageBreak(false, marginBottom)
	pdf.SetTitle(title, true)
	pdf.SetCreator("phytozome GO", true)
	pdf.AliasNbPages("")

	r := &pdfReportRenderer{
		doc: &pdfDocument{title: title, generatedAt: generatedAt, pdf: pdf},
	}
	r.doc.fontErr = r.doc.ensureSystemFonts()
	r.addPage("Cover")
	return r
}

func (r *pdfReportRenderer) save(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("report path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create report directory: %w", err)
	}
	if err := r.doc.ensureSystemFonts(); err != nil {
		return err
	}
	r.doc.pdf.SetFooterFunc(func() {
		r.doc.drawFooter()
	})
	return r.doc.pdf.OutputFileAndClose(path)
}

func (d *pdfDocument) ensureSystemFonts() error {
	if d.fontsLoaded {
		return nil
	}
	if d.fontErr != nil {
		return d.fontErr
	}
	info, err := resolveSystemReportFont()
	if err != nil {
		d.fontErr = err
		return err
	}
	d.pdf.AddUTF8FontFromBytes(reportSansFamily, "", info.RegularBytes)
	d.pdf.AddUTF8FontFromBytes(reportSansFamily, "B", info.BoldBytes)
	if err := d.pdf.Error(); err != nil {
		d.fontErr = fmt.Errorf("load system report font %s: %w", info.Description, err)
		return d.fontErr
	}
	d.fontInfo = info
	d.fontsLoaded = true
	return nil
}

func (r *pdfReportRenderer) addPage(section string) {
	r.doc.pdf.AddPage()
	r.y = marginTop
	r.drawHeader(section)
}

func (r *pdfReportRenderer) drawHeader(section string) {
	titleLines := wrapText(r.doc.title, 305, 8.5)
	sectionLines := wrapText(section, 180, 8.5)
	lineHeight := 10.5
	for i, line := range titleLines {
		r.text(marginLeft, 22+float64(i)*lineHeight, 8.5, fontBold, colorMuted, line)
	}
	for i, line := range sectionLines {
		r.text(pageWidth-marginRight-190, 22+float64(i)*lineHeight, 8.5, fontRegular, colorMuted, line)
	}
	lines := math.Max(float64(len(titleLines)), float64(len(sectionLines)))
	ruleY := 30 + lines*lineHeight
	if ruleY < 42 {
		ruleY = 42
	}
	r.line(marginLeft, ruleY, pageWidth-marginRight, ruleY, colorRule, 0.6)
	r.y = ruleY + 12
}

func (r *pdfReportRenderer) ensure(height float64) {
	if r.y+height <= pageHeight-marginBottom {
		return
	}
	section := r.chapter
	if section == "" {
		section = "Continued"
	}
	r.addPage(section)
}

func (r *pdfReportRenderer) title(title string, subtitle string) {
	r.y = 104
	lines := wrapText(title, pageWidth-marginLeft-marginRight, 24)
	for _, line := range lines {
		r.text(marginLeft, r.y, 24, fontBold, colorText, line)
		r.y += 30
	}
	r.y += 2
	if subtitle != "" {
		r.paragraph(subtitle, 11, colorMuted, pageWidth-marginLeft-marginRight)
	}
	r.y += 16
	r.line(marginLeft, r.y, pageWidth-marginRight, r.y, colorPrimary, 1.2)
	r.y += 20
}

func (r *pdfReportRenderer) chapterHeading(title string) {
	r.chapter = title
	lines := wrapText(title, pageWidth-marginLeft-marginRight, 15)
	lineHeight := 19.0
	r.ensure(50 + float64(len(lines))*lineHeight)
	if r.y < marginTop+42 {
		r.y = marginTop + 42
	} else {
		r.y += 24
	}
	for _, line := range lines {
		r.text(marginLeft, r.y, 15, fontBold, colorPrimary, line)
		r.y += lineHeight
	}
	r.doc.pdf.Bookmark(title, 0, -1)
	r.y += 2
	r.line(marginLeft, r.y, pageWidth-marginRight, r.y, colorRule, 0.5)
	r.y += 14
}

func (r *pdfReportRenderer) subheading(title string) {
	lines := wrapText(title, pageWidth-marginLeft-marginRight, 11)
	lineHeight := 14.5
	r.ensure(float64(len(lines))*lineHeight + 4)
	for _, line := range lines {
		r.text(marginLeft, r.y, 11, fontBold, colorText, line)
		r.y += lineHeight
	}
	r.y += 2
}

func (r *pdfReportRenderer) paragraph(text string, size float64, color pdfColor, width float64) {
	lines := wrapText(text, width, size)
	lineHeight := size * 1.38
	for _, line := range lines {
		r.ensure(lineHeight + 4)
		r.text(marginLeft, r.y, size, fontRegular, color, line)
		r.y += lineHeight
	}
	r.y += 5
}

func (r *pdfReportRenderer) note(text string) {
	lines := wrapText(text, pageWidth-marginLeft-marginRight-18, 8.5)
	for len(lines) > 0 {
		availableLines := int((pageHeight - marginBottom - r.y - 14) / 11.5)
		if availableLines < 1 {
			r.addPage(r.chapter)
			availableLines = int((pageHeight - marginBottom - r.y - 14) / 11.5)
		}
		if availableLines > len(lines) {
			availableLines = len(lines)
		}
		chunk := lines[:availableLines]
		lines = lines[availableLines:]
		height := math.Max(32, float64(len(chunk))*11.5+14)
		r.ensure(height)
		r.rect(marginLeft, r.y, pageWidth-marginLeft-marginRight, height, pdfColor{0.94, 0.97, 0.98}, pdfColor{0.74, 0.85, 0.90}, 0.4)
		y := r.y + 10
		for _, line := range chunk {
			r.text(marginLeft+9, y, 8.5, fontRegular, colorMuted, line)
			y += 11.5
		}
		r.y += height
	}
}

func (r *pdfReportRenderer) cards(cards []NameValue) {
	if len(cards) == 0 {
		return
	}
	cardW := (pageWidth - marginLeft - marginRight - 20) / 3
	for rowStart := 0; rowStart < len(cards); rowStart += 3 {
		rowEnd := rowStart + 3
		if rowEnd > len(cards) {
			rowEnd = len(cards)
		}
		type cardLayout struct {
			name        []string
			value       []string
			explanation []string
			height      float64
		}
		layouts := make([]cardLayout, rowEnd-rowStart)
		rowH := 58.0
		for i := rowStart; i < rowEnd; i++ {
			card := cards[i]
			layout := cardLayout{
				name:        wrapText(strings.ToUpper(card.Name), cardW-18, 7.5),
				value:       wrapText(card.Value, cardW-18, 13),
				explanation: wrapText(card.Explanation, cardW-18, 7.5),
			}
			layout.height = 12 + float64(len(layout.name))*9.5 + 5 + float64(len(layout.value))*15.5
			if strings.TrimSpace(card.Explanation) != "" {
				layout.height += 5 + float64(len(layout.explanation))*9.8
			}
			layout.height += 9
			if layout.height > rowH {
				rowH = layout.height
			}
			layouts[i-rowStart] = layout
		}
		r.ensure(rowH + 12)
		y := r.y
		for i := rowStart; i < rowEnd; i++ {
			x := marginLeft + float64(i-rowStart)*(cardW+10)
			layout := layouts[i-rowStart]
			r.rect(x, y, cardW, rowH, colorFill, colorRule, 0.5)
			lineY := y + 14
			for _, line := range layout.name {
				r.text(x+9, lineY, 7.5, fontBold, colorMuted, line)
				lineY += 9.5
			}
			lineY += 3
			for _, line := range layout.value {
				r.text(x+9, lineY, 13, fontBold, colorPrimary, line)
				lineY += 15.5
			}
			if len(layout.explanation) > 0 && strings.TrimSpace(cards[i].Explanation) != "" {
				lineY += 3
				for _, line := range layout.explanation {
					r.text(x+9, lineY, 7.5, fontRegular, colorMuted, line)
					lineY += 9.8
				}
			}
		}
		r.y += rowH + 12
	}
}

func (r *pdfReportRenderer) table(headers []string, rows [][]string, widths []float64) {
	if len(headers) == 0 || len(widths) != len(headers) {
		return
	}
	estimateHeaderHeight := func() float64 {
		maxLines := 1
		for i, header := range headers {
			lines := wrapText(header, widths[i]-8, 7.3)
			if len(lines) > maxLines {
				maxLines = len(lines)
			}
		}
		return math.Max(20, float64(maxLines)*9.5+8)
	}
	estimateFirstRowHeight := func() float64 {
		if len(rows) == 0 {
			return 18
		}
		maxLines := 1
		for i := range headers {
			value := ""
			if i < len(rows[0]) {
				value = rows[0][i]
			}
			lines := wrapText(value, widths[i]-8, 7.4)
			if len(lines) > maxLines {
				maxLines = len(lines)
			}
		}
		return math.Max(18, float64(maxLines)*9.8+8)
	}
	minTableStart := estimateHeaderHeight() + estimateFirstRowHeight()
	if r.y+minTableStart > pageHeight-marginBottom {
		r.addPage(r.chapter)
	}
	renderHeader := func() {
		headerLines := make([][]string, len(headers))
		maxLines := 1
		for i, header := range headers {
			headerLines[i] = wrapText(header, widths[i]-8, 7.3)
			if len(headerLines[i]) > maxLines {
				maxLines = len(headerLines[i])
			}
		}
		headerH := math.Max(20, float64(maxLines)*9.5+8)
		r.ensure(headerH)
		x := marginLeft
		for i, header := range headers {
			_ = header
			r.rect(x, r.y, widths[i], headerH, colorHeader, colorRule, 0.35)
			lineY := r.y + 12
			for _, line := range headerLines[i] {
				r.text(x+4, lineY, 7.3, fontBold, colorText, line)
				lineY += 9.5
			}
			x += widths[i]
		}
		r.y += headerH
	}
	renderHeader()
	for _, row := range rows {
		cellLines := make([][]string, len(headers))
		maxLines := 1
		for i := range headers {
			value := ""
			if i < len(row) {
				value = row[i]
			}
			cellLines[i] = wrapText(value, widths[i]-8, 7.4)
			if len(cellLines[i]) > maxLines {
				maxLines = len(cellLines[i])
			}
		}
		minRowH := math.Max(18, 1*9.8+8)
		if r.y+minRowH > pageHeight-marginBottom {
			r.addPage(r.chapter)
			renderHeader()
		}
		lineStart := 0
		for lineStart < maxLines {
			availableH := pageHeight - marginBottom - r.y
			if availableH < 28 {
				r.addPage(r.chapter)
				renderHeader()
				availableH = pageHeight - marginBottom - r.y
			}
			maxChunkLines := int((availableH - 8) / 9.8)
			if maxChunkLines < 1 {
				r.addPage(r.chapter)
				renderHeader()
				maxChunkLines = int((pageHeight - marginBottom - r.y - 8) / 9.8)
			}
			lineEnd := lineStart + maxChunkLines
			if lineEnd > maxLines {
				lineEnd = maxLines
			}
			chunkLines := lineEnd - lineStart
			rowH := math.Max(18, float64(chunkLines)*9.8+8)
			x := marginLeft
			for i := range headers {
				r.rect(x, r.y, widths[i], rowH, pdfColor{1, 1, 1}, colorRule, 0.25)
				lineY := r.y + 11
				for j := lineStart; j < lineEnd && j < len(cellLines[i]); j++ {
					r.text(x+4, lineY, 7.4, fontRegular, colorText, cellLines[i][j])
					lineY += 9.8
				}
				x += widths[i]
			}
			r.y += rowH
			lineStart = lineEnd
		}
	}
	r.y += 12
}

func (r *pdfReportRenderer) selectionChart(sel KeywordSelectionStats) {
	total := sel.TotalRows
	if total <= 0 {
		r.note("Selection chart was not rendered because the current report data contains no result rows.")
		return
	}
	r.ensure(190)
	top := r.y
	r.text(marginLeft, top, 10, fontBold, colorText, "Selected rows versus all current rows")
	segments := []chartSegment{
		{Label: "selected/exported", Value: float64(sel.SelectedRows), Color: colorPrimary},
		{Label: "unselected", Value: float64(sel.UnselectedRows), Color: colorSecondary},
	}
	r.donut(marginLeft+76, top+82, 58, 28, segments)
	r.text(marginLeft+58, top+86, 12, fontBold, colorText, fmt.Sprintf("%d", total))
	r.text(marginLeft+51, top+100, 7.3, fontRegular, colorMuted, "total rows")
	legendX := marginLeft + 160
	r.legend(legendX, top+34, segments)
	r.y = math.Max(top+154, top+34+r.legendHeight(pageWidth-marginRight-legendX, segments)+12)
	r.paragraph("The chart shows the proportion of the current keyword result table that was included in the export. Selection is an audit description of user choice; it is not interpreted as biological quality.", 8.8, colorMuted, pageWidth-marginLeft-marginRight)
}

func (r *pdfReportRenderer) termHitChart(sel KeywordSelectionStats) {
	total := sel.SearchTerms
	if total <= 0 {
		r.note("Search-term hit chart was not rendered because no search terms were recorded.")
		return
	}
	r.ensure(170)
	top := r.y
	r.text(marginLeft, top, 10, fontBold, colorText, "Search terms with data versus zero-hit search terms")
	segments := []chartSegment{
		{Label: "terms with data", Value: float64(sel.TermsWithHits), Color: colorSuccess},
		{Label: "zero-hit terms", Value: float64(sel.TermsZeroHits), Color: colorWarning},
	}
	r.donut(marginLeft+76, top+78, 54, 26, segments)
	r.text(marginLeft+60, top+82, 12, fontBold, colorText, fmt.Sprintf("%d", total))
	r.text(marginLeft+48, top+96, 7.3, fontRegular, colorMuted, "search terms")
	legendX := marginLeft + 160
	r.legend(legendX, top+36, segments)
	r.y = math.Max(top+138, top+36+r.legendHeight(pageWidth-marginRight-legendX, segments)+12)
	r.paragraph("This chart is term-based rather than row-based. A search term is counted as having data when the current keyword workflow returned at least one result row for that term; it is counted as zero-hit when no rows were returned.", 8.8, colorMuted, pageWidth-marginLeft-marginRight)
}

func (r *pdfReportRenderer) provenanceChart(parts []ProvenanceSlice) {
	var total int
	for _, part := range parts {
		total += part.Count
	}
	if total <= 0 {
		r.note("Provenance chart was not rendered because no provenance counts were available in this run.")
		return
	}
	colors := []pdfColor{colorPrimary, colorSuccess, colorPurple, colorWarning, colorSecondary, colorMissing}
	segments := make([]chartSegment, 0, len(parts))
	for i, part := range parts {
		segments = append(segments, chartSegment{Label: part.Label, Value: float64(part.Count), Color: colors[i%len(colors)]})
	}
	r.ensure(190)
	top := r.y
	r.text(marginLeft, top, 10, fontBold, colorText, "Data provenance distribution")
	r.donut(marginLeft+76, top+82, 58, 28, segments)
	legendX := marginLeft + 160
	r.legend(legendX, top+25, segments)
	r.y = math.Max(top+154, top+25+r.legendHeight(pageWidth-marginRight-legendX, segments)+12)
	r.paragraph("This chart separates values that came directly from the selected source, values parsed from local release assets, values backed by cache state, values generated internally, and values unavailable in this run.", 8.8, colorMuted, pageWidth-marginLeft-marginRight)
}

func (r *pdfReportRenderer) tableCompletenessChart(columns []ColumnCompleteness) {
	filled := 0
	empty := 0
	for _, col := range columns {
		filled += col.FilledRows
		empty += col.EmptyRows
	}
	total := filled + empty
	if total <= 0 {
		r.note("Table completeness chart was not rendered because no generated table cells were available.")
		return
	}
	r.ensure(170)
	top := r.y
	r.text(marginLeft, top, 10, fontBold, colorText, "Generated table cell completeness")
	segments := []chartSegment{
		{Label: "cells with data", Value: float64(filled), Color: colorSuccess},
		{Label: "empty cells", Value: float64(empty), Color: colorMissing},
	}
	r.donut(marginLeft+76, top+78, 54, 26, segments)
	r.text(marginLeft+54, top+82, 12, fontBold, colorText, fmt.Sprintf("%d", total))
	r.text(marginLeft+54, top+96, 7.3, fontRegular, colorMuted, "table cells")
	legendX := marginLeft + 160
	r.legend(legendX, top+36, segments)
	r.y = math.Max(top+138, top+36+r.legendHeight(pageWidth-marginRight-legendX, segments)+12)
	r.paragraph("This quality chart is computed from the keyword table columns and rows that the export writes. It does not include the PDF report file, report metadata, system information, or fields that are not part of the generated keyword table.", 8.8, colorMuted, pageWidth-marginLeft-marginRight)
}

func (r *pdfReportRenderer) termBars(terms []KeywordTermReport) {
	if len(terms) <= 1 {
		return
	}
	r.ensure(38)
	r.text(marginLeft, r.y, 10, fontBold, colorText, "Per-term selected and unselected row counts")
	r.y += 18
	maxTotal := 1
	for _, term := range terms {
		if term.TotalRows > maxTotal {
			maxTotal = term.TotalRows
		}
	}
	labelW := 115.0
	barW := pageWidth - marginLeft - marginRight - labelW - 16
	for _, term := range terms {
		labelLines := wrapText(term.SearchTerm, labelW-4, 7.8)
		rowH := math.Max(23, float64(len(labelLines))*9.6+5)
		r.ensure(rowH)
		y := r.y
		lineY := y + 9
		for _, line := range labelLines {
			r.text(marginLeft, lineY, 7.8, fontRegular, colorText, line)
			lineY += 9.6
		}
		x := marginLeft + labelW
		r.rect(x, y, barW, 11, pdfColor{0.94, 0.95, 0.96}, colorRule, 0.2)
		selectedW := barW * float64(term.SelectedRows) / float64(maxTotal)
		unselectedW := barW * float64(term.TotalRows-term.SelectedRows) / float64(maxTotal)
		if selectedW > 0 {
			r.fillRect(x, y, selectedW, 11, colorPrimary)
		}
		if unselectedW > 0 {
			r.fillRect(x+selectedW, y, unselectedW, 11, colorSecondary)
		}
		r.text(x+barW+5, y+9, 7.2, fontRegular, colorMuted, fmt.Sprintf("%d/%d", term.SelectedRows, term.TotalRows))
		r.y += rowH
	}
	r.y += 8
}

func (r *pdfReportRenderer) durationBars(steps []GenerationStep) {
	if len(steps) == 0 {
		r.note("Duration chart was not rendered because no generation steps were instrumented.")
		return
	}
	total := int64(0)
	for _, step := range steps {
		total += stepDuration(step)
	}
	if total <= 0 {
		r.note("Duration chart was not rendered because generation step timestamps were not available.")
		return
	}
	r.ensure(96)
	r.text(marginLeft, r.y, 10, fontBold, colorText, "Measured generation time by step")
	r.y += 18
	colors := []pdfColor{colorPrimary, colorSuccess, colorPurple, colorWarning, colorSecondary, colorMissing}
	barW := pageWidth - marginLeft - marginRight
	r.rect(marginLeft, r.y, barW, 16, pdfColor{0.96, 0.97, 0.98}, colorRule, 0.25)
	x := marginLeft
	for i, step := range steps {
		w := barW * float64(stepDuration(step)) / float64(total)
		if w > 0 && w < 1.2 {
			w = 1.2
		}
		if x+w > marginLeft+barW {
			w = marginLeft + barW - x
		}
		if w > 0 {
			r.fillRect(x, r.y, w, 16, colors[i%len(colors)])
		}
		x += w
	}
	r.strokeRect(marginLeft, r.y, barW, 16, colorRule, 0.35)
	r.y += 28
	legendSegments := make([]chartSegment, 0, len(steps))
	for i, step := range steps {
		legendSegments = append(legendSegments, chartSegment{
			Label: fmt.Sprintf("%s (%s)", step.Name, formatDurationMS(stepDuration(step))),
			Value: float64(stepDuration(step)),
			Color: colors[i%len(colors)],
		})
	}
	r.legendBox(marginLeft, r.y, barW, legendSegments)
}

type chartSegment struct {
	Label string
	Value float64
	Color pdfColor
}

func (r *pdfReportRenderer) legend(x float64, y float64, segments []chartSegment) {
	r.legendAt(x, y, pageWidth-marginRight-x, segments)
}

func (r *pdfReportRenderer) legendBox(x float64, y float64, width float64, segments []chartSegment) {
	r.y = y
	r.ensure(r.legendHeight(width, segments))
	r.legendAt(x, r.y, width, segments)
	r.y += r.legendHeight(width, segments)
	r.y += 8
}

func (r *pdfReportRenderer) legendAt(x float64, y float64, width float64, segments []chartSegment) {
	total := 0.0
	for _, segment := range segments {
		total += segment.Value
	}
	textW := width - 15
	if textW < 40 {
		textW = 40
	}
	for _, segment := range segments {
		percent := 0.0
		if total > 0 {
			percent = segment.Value / total * 100
		}
		label := fmt.Sprintf("%s: %.0f (%.1f%%)", segment.Label, segment.Value, percent)
		lines := wrapText(label, textW, 7.6)
		needed := math.Max(13, float64(len(lines))*9.2+2)
		r.fillRect(x, y-7, 8, 8, segment.Color)
		lineY := y
		for _, line := range lines {
			r.text(x+13, lineY, 7.6, fontRegular, colorText, line)
			lineY += 9.2
		}
		y += needed
	}
}

func (r *pdfReportRenderer) legendHeight(width float64, segments []chartSegment) float64 {
	textW := width - 15
	if textW < 40 {
		textW = 40
	}
	height := 0.0
	for _, segment := range segments {
		label := fmt.Sprintf("%s: %.0f (%.1f%%)", segment.Label, segment.Value, 100.0)
		lines := wrapText(label, textW, 7.6)
		height += math.Max(13, float64(len(lines))*9.2+2)
	}
	return height
}

func (r *pdfReportRenderer) donut(cx float64, cy float64, outer float64, inner float64, segments []chartSegment) {
	total := 0.0
	for _, segment := range segments {
		total += segment.Value
	}
	if total <= 0 {
		return
	}
	start := -math.Pi / 2
	for _, segment := range segments {
		if segment.Value <= 0 {
			continue
		}
		end := start + 2*math.Pi*(segment.Value/total)
		r.annularSegment(cx, cy, outer, inner, start, end, segment.Color)
		start = end
	}
	r.circle(cx, cy, inner-1, pdfColor{1, 1, 1})
}

func (r *pdfReportRenderer) annularSegment(cx, cy, outer, inner, start, end float64, color pdfColor) {
	steps := int(math.Max(8, math.Ceil((end-start)/(math.Pi/18))))
	points := make([]gofpdf.PointType, 0, steps*2+2)
	for i := 0; i <= steps; i++ {
		a := start + (end-start)*float64(i)/float64(steps)
		points = append(points, gofpdf.PointType{X: cx + math.Cos(a)*outer, Y: cy + math.Sin(a)*outer})
	}
	for i := steps; i >= 0; i-- {
		a := start + (end-start)*float64(i)/float64(steps)
		points = append(points, gofpdf.PointType{X: cx + math.Cos(a)*inner, Y: cy + math.Sin(a)*inner})
	}
	pdf := r.doc.pdf
	setFillColor(pdf, color)
	setDrawColor(pdf, color)
	pdf.Polygon(points, "F")
}

func (r *pdfReportRenderer) circle(cx, cy, radius float64, color pdfColor) {
	pdf := r.doc.pdf
	setFillColor(pdf, color)
	setDrawColor(pdf, color)
	pdf.Circle(cx, cy, radius, "F")
}

func (r *pdfReportRenderer) text(x, yTop, size float64, font string, color pdfColor, text string) {
	if r.doc.fontErr != nil {
		return
	}
	pdf := r.doc.pdf
	style := ""
	offset := 0.10
	if font == fontBold {
		style = "B"
		offset = 0.16
	}
	setTextColor(pdf, color)
	pdf.SetFont(reportSansFamily, style, size)
	pdf.Text(x, yTop, text)
	if size <= 12 {
		pdf.Text(x+offset, yTop, text)
	}
	if font == fontBold {
		pdf.Text(x, yTop+0.08, text)
	}
}

func (r *pdfReportRenderer) line(x1, y1Top, x2, y2Top float64, color pdfColor, width float64) {
	pdf := r.doc.pdf
	setDrawColor(pdf, color)
	pdf.SetLineWidth(width)
	pdf.Line(x1, y1Top, x2, y2Top)
}

func (r *pdfReportRenderer) fillRect(x, yTop, w, h float64, color pdfColor) {
	pdf := r.doc.pdf
	setFillColor(pdf, color)
	setDrawColor(pdf, color)
	pdf.Rect(x, yTop, w, h, "F")
}

func (r *pdfReportRenderer) rect(x, yTop, w, h float64, fill pdfColor, stroke pdfColor, strokeW float64) {
	pdf := r.doc.pdf
	setFillColor(pdf, fill)
	setDrawColor(pdf, stroke)
	pdf.SetLineWidth(strokeW)
	pdf.Rect(x, yTop, w, h, "FD")
}

func (r *pdfReportRenderer) strokeRect(x, yTop, w, h float64, stroke pdfColor, strokeW float64) {
	pdf := r.doc.pdf
	setDrawColor(pdf, stroke)
	pdf.SetLineWidth(strokeW)
	pdf.Rect(x, yTop, w, h, "D")
}

func (d *pdfDocument) drawFooter() {
	pdf := d.pdf
	setDrawColor(pdf, colorRule)
	pdf.SetLineWidth(0.5)
	pdf.Line(marginLeft, pageHeight-32, pageWidth-marginRight, pageHeight-32)
	setTextColor(pdf, colorMuted)
	pdf.SetFont(reportSansFamily, "", 7.5)
	left := fmt.Sprintf("Generated %s", d.generatedAt.Local().Format("2006-01-02 15:04:05 MST"))
	right := fmt.Sprintf("Page %d of {nb}", pdf.PageNo())
	pdf.Text(marginLeft, pageHeight-19, left)
	pdf.Text(pageWidth-marginRight-62, pageHeight-19, right)
}

func setFillColor(pdf *gofpdf.Fpdf, color pdfColor) {
	pdf.SetFillColor(colorByte(color.R), colorByte(color.G), colorByte(color.B))
}

func setDrawColor(pdf *gofpdf.Fpdf, color pdfColor) {
	pdf.SetDrawColor(colorByte(color.R), colorByte(color.G), colorByte(color.B))
}

func setTextColor(pdf *gofpdf.Fpdf, color pdfColor) {
	pdf.SetTextColor(colorByte(color.R), colorByte(color.G), colorByte(color.B))
}

func colorByte(value float64) int {
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}
	return int(math.Round(value * 255))
}

type systemFontInfo struct {
	RegularBytes []byte
	BoldBytes    []byte
	Description  string
}

func resolveSystemReportFont() (systemFontInfo, error) {
	regularPath, regularBytes := firstReadableFont(systemSansRegularCandidates())
	if regularPath == "" {
		return systemFontInfo{}, fmt.Errorf("no readable system sans/CJK font was found")
	}
	boldPath, boldBytes := firstReadableFont(systemSansBoldCandidates())
	if boldPath == "" {
		boldPath = regularPath
		boldBytes = regularBytes
	}
	return systemFontInfo{
		RegularBytes: regularBytes,
		BoldBytes:    boldBytes,
		Description:  fmt.Sprintf("regular=%s; bold=%s", regularPath, boldPath),
	}, nil
}

func firstReadableFont(paths []string) (string, []byte) {
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil || len(data) == 0 {
			continue
		}
		if face, err := firstFontFaceBytes(data); err == nil && len(face) > 0 {
			return path, face
		}
		return path, data
	}
	return "", nil
}

func systemSansRegularCandidates() []string {
	return []string{
		`C:\Windows\Fonts\msyh.ttc`,
		`C:\Windows\Fonts\Deng.ttf`,
		`C:\Windows\Fonts\malgun.ttf`,
		`C:\Windows\Fonts\meiryo.ttc`,
		`C:\Windows\Fonts\simhei.ttf`,
		"/System/Library/Fonts/PingFang.ttc",
		"/System/Library/Fonts/Hiragino Sans GB.ttc",
		"/System/Library/Fonts/Supplemental/Arial Unicode.ttf",
		"/System/Library/Fonts/Supplemental/Arial Unicode MS.ttf",
		"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.otf",
		"/usr/share/fonts/opentype/source-han-sans/SourceHanSansCN-Regular.otf",
		"/usr/share/fonts/adobe-source-han-sans/SourceHanSansCN-Regular.otf",
		"/usr/share/fonts/truetype/wqy/wqy-microhei.ttc",
	}
}

func systemSansBoldCandidates() []string {
	return []string{
		`C:\Windows\Fonts\msyhbd.ttc`,
		`C:\Windows\Fonts\Dengb.ttf`,
		`C:\Windows\Fonts\malgunbd.ttf`,
		`C:\Windows\Fonts\meiryob.ttc`,
		`C:\Windows\Fonts\simhei.ttf`,
		"/System/Library/Fonts/PingFang.ttc",
		"/System/Library/Fonts/Hiragino Sans GB.ttc",
		"/System/Library/Fonts/Supplemental/Arial Bold.ttf",
		"/usr/share/fonts/opentype/noto/NotoSansCJK-Bold.ttc",
		"/usr/share/fonts/truetype/noto/NotoSansCJK-Bold.ttc",
		"/usr/share/fonts/opentype/noto/NotoSansCJK-Bold.otf",
		"/usr/share/fonts/opentype/source-han-sans/SourceHanSansCN-Bold.otf",
		"/usr/share/fonts/adobe-source-han-sans/SourceHanSansCN-Bold.otf",
		"/usr/share/fonts/truetype/wqy/wqy-microhei.ttc",
	}
}

func firstFontFaceBytes(data []byte) ([]byte, error) {
	if len(data) < 12 || string(data[:4]) != "ttcf" {
		return nil, fmt.Errorf("not a TrueType collection")
	}
	numFonts := int(u32(data, 8))
	if numFonts <= 0 || len(data) < 12+numFonts*4 {
		return nil, fmt.Errorf("invalid TrueType collection")
	}
	offset := int(u32(data, 12))
	return extractFontFace(data, offset)
}

func extractFontFace(data []byte, offset int) ([]byte, error) {
	if offset < 0 || offset+12 > len(data) {
		return nil, fmt.Errorf("invalid font face offset")
	}
	numTables := int(u16(data, offset+4))
	dirLen := 12 + numTables*16
	if numTables <= 0 || offset+dirLen > len(data) {
		return nil, fmt.Errorf("invalid font face directory")
	}
	type tableRecord struct {
		tag      []byte
		checksum []byte
		offset   int
		length   int
		newOff   int
	}
	records := make([]tableRecord, 0, numTables)
	outLen := dirLen
	for i := 0; i < numTables; i++ {
		pos := offset + 12 + i*16
		tableOffset := int(u32(data, pos+8))
		tableLength := int(u32(data, pos+12))
		if tableOffset < 0 || tableLength < 0 || tableOffset+tableLength > len(data) {
			return nil, fmt.Errorf("invalid font table bounds")
		}
		if rem := outLen % 4; rem != 0 {
			outLen += 4 - rem
		}
		records = append(records, tableRecord{
			tag:      append([]byte(nil), data[pos:pos+4]...),
			checksum: append([]byte(nil), data[pos+4:pos+8]...),
			offset:   tableOffset,
			length:   tableLength,
			newOff:   outLen,
		})
		outLen += tableLength
	}
	out := make([]byte, outLen)
	copy(out[:12], data[offset:offset+12])
	for i, rec := range records {
		pos := 12 + i*16
		copy(out[pos:pos+4], rec.tag)
		copy(out[pos+4:pos+8], rec.checksum)
		putU32(out, pos+8, uint32(rec.newOff))
		putU32(out, pos+12, uint32(rec.length))
		copy(out[rec.newOff:rec.newOff+rec.length], data[rec.offset:rec.offset+rec.length])
	}
	return out, nil
}

func u16(data []byte, offset int) uint16 {
	return uint16(data[offset])<<8 | uint16(data[offset+1])
}

func u32(data []byte, offset int) uint32 {
	return uint32(data[offset])<<24 | uint32(data[offset+1])<<16 | uint32(data[offset+2])<<8 | uint32(data[offset+3])
}

func putU32(data []byte, offset int, value uint32) {
	data[offset] = byte(value >> 24)
	data[offset+1] = byte(value >> 16)
	data[offset+2] = byte(value >> 8)
	data[offset+3] = byte(value)
}

func wrapText(s string, width float64, size float64) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	parts := strings.Split(s, "\n")
	var out []string
	for _, part := range parts {
		lines := wrapTextLine(part, width, size)
		out = append(out, lines...)
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

func wrapTextLine(s string, width float64, size float64) []string {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return []string{""}
	}
	maxUnits := int(width / (size * 0.52))
	if maxUnits < 8 {
		maxUnits = 8
	}
	words := strings.Fields(s)
	var lines []string
	var current string
	for _, word := range words {
		for visualWidth(word) > maxUnits {
			if current != "" {
				lines = append(lines, current)
				current = ""
			}
			left, right := splitByVisualWidth(word, maxUnits-1)
			lines = append(lines, left)
			word = right
		}
		if current == "" {
			current = word
			continue
		}
		if visualWidth(current)+1+visualWidth(word) <= maxUnits {
			current += " " + word
			continue
		}
		lines = append(lines, current)
		current = word
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func splitByVisualWidth(s string, maxUnits int) (string, string) {
	if maxUnits <= 0 {
		_, size := utf8.DecodeRuneInString(s)
		if size <= 0 {
			return "", ""
		}
		return s[:size], s[size:]
	}
	used := 0
	for i, r := range s {
		w := runeVisualWidth(r)
		if used+w > maxUnits {
			if i == 0 {
				_, size := utf8.DecodeRuneInString(s)
				return s[:size], s[size:]
			}
			return s[:i], s[i:]
		}
		used += w
	}
	return s, ""
}

func visualWidth(s string) int {
	width := 0
	for _, r := range s {
		width += runeVisualWidth(r)
	}
	return width
}

func runeVisualWidth(r rune) int {
	switch {
	case r == '\t':
		return 4
	case r >= 0x1100 && r <= 0x115F:
		return 2
	case r >= 0x2E80 && r <= 0xA4CF:
		return 2
	case r >= 0xAC00 && r <= 0xD7A3:
		return 2
	case r >= 0xF900 && r <= 0xFAFF:
		return 2
	case r >= 0xFE10 && r <= 0xFE6F:
		return 2
	case r >= 0xFF00 && r <= 0xFF60:
		return 2
	case r >= 0xFFE0 && r <= 0xFFE6:
		return 2
	default:
		return 1
	}
}

func truncate(s string, max int) string {
	if max <= 0 || visualWidth(s) <= max {
		return s
	}
	if max < 4 {
		var b strings.Builder
		for _, r := range s {
			if visualWidth(b.String())+runeVisualWidth(r) > max {
				break
			}
			b.WriteRune(r)
		}
		return b.String()
	}
	left, _ := splitByVisualWidth(s, max-3)
	return left + "..."
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "not available in this run"
	}
	return t.Local().Format("2006-01-02 15:04:05.000 MST")
}

func formatDurationMS(ms int64) string {
	if ms < 0 {
		return "not available"
	}
	if ms < 1000 {
		return fmt.Sprintf("%d ms", ms)
	}
	return fmt.Sprintf("%.2f s", float64(ms)/1000)
}

func stepDuration(step GenerationStep) int64 {
	if step.DurationMS > 0 {
		return step.DurationMS
	}
	if !step.Start.IsZero() && !step.End.IsZero() {
		return step.End.Sub(step.Start).Milliseconds()
	}
	return 0
}

func fileSizeText(n int64) string {
	if n < 0 {
		return "not available before final PDF is written"
	}
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
}

func sortedRowsFromNameValues(values []NameValue) [][]string {
	rows := make([][]string, 0, len(values))
	for _, value := range values {
		rows = append(rows, []string{value.Name, value.Value, value.Explanation})
	}
	return rows
}

func sortedProvenanceRows(parts []ProvenanceSlice) [][]string {
	rows := make([][]string, 0, len(parts))
	for _, part := range parts {
		rows = append(rows, []string{part.Label, fmt.Sprintf("%d", part.Count), part.Explanation})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i][0] < rows[j][0]
	})
	return rows
}

func pdfBytesForTest(data ReportData) ([]byte, error) {
	r := newPDFReport(data.Title, data.GeneratedAt)
	renderKeywordReport(r, data)
	if err := r.doc.ensureSystemFonts(); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	r.doc.pdf.SetFooterFunc(func() {
		r.doc.drawFooter()
	})
	err := r.doc.pdf.Output(&out)
	return out.Bytes(), err
}
