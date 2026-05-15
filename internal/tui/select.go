// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package tui

import (
	"errors"
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"io"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
)

var ErrBack = errors.New("tui back requested")

var helpLanguageIndex atomic.Int32

const (
	colorCanvas       = tcell.ColorBlack
	colorPanel        = tcell.ColorDarkBlue
	colorAction       = tcell.ColorDeepSkyBlue
	colorActionText   = tcell.ColorBlack
	colorText         = tcell.ColorGhostWhite
	colorMuted        = tcell.ColorYellow
	colorAccent       = tcell.ColorGreen
	colorSelectionOn  = tcell.ColorGreen
	colorSelectionOff = tcell.ColorRed
	colorBorder       = tcell.ColorWhite
	colorInactiveText = tcell.ColorDarkCyan
)

type Option struct {
	Value       string
	Label       string
	Description string
}

type StartupInfo struct {
	DisplayName string
	Version     string
	Author      string
	RepoURL     string
	LicenseName string
	LicenseID   string
}

type StartupChoice struct {
	Database string
	Mode     string
	Tool     string
}

func SelectStartup(in io.Reader, out io.Writer, info StartupInfo) (StartupChoice, error) {
	features := []Option{
		{Value: "keyword", Label: "Keyword", Description: "search annotations, IDs, aliases, or descriptions"},
		{Value: "blast", Label: "Blast", Description: "sequence / FASTA / URL query against one species"},
		{Value: "tools", Label: "Tools", Description: "standalone analysis and helper workflows"},
	}
	subOptions := map[string][]Option{
		"keyword": {
			{Value: "phytozome:keyword", Label: "Phytozome keyword", Description: "keyword search in Phytozome species"},
			{Value: "lemna:keyword", Label: "lemna keyword", Description: "keyword search in lemna.org releases"},
		},
		"blast": {
			{Value: "phytozome:blast", Label: "Phytozome blast", Description: "BLAST against a selected Phytozome species"},
			{Value: "lemna:blast", Label: "lemna blast", Description: "BLAST against lemna.org releases"},
		},
		"tools": {
			{Value: "tool:pathway_search", Label: "Pathway search", Description: "pathway search entry point; implementation comes next"},
		},
	}

	app := newApp()
	var result StartupChoice

	featureList := optionListWithStart("Function selection:", features, 1)
	subOptionList := optionListWithStart("Sub-option selection:", subOptions[features[0].Value], 4)
	selectedFeatureIndex := 0
	selectedFeature := func() Option {
		featureIndex := selectedFeatureIndex
		if featureIndex < 0 || featureIndex >= len(features) {
			featureIndex = 0
		}
		return features[featureIndex]
	}
	currentSubOptions := func() []Option {
		return subOptions[selectedFeature().Value]
	}
	refreshSubOptions := func() {
		setOptionListItems(subOptionList, "Sub-option selection:", currentSubOptions(), 4)
	}
	selectFeature := func(index int) {
		if index < 0 || index >= len(features) {
			index = 0
		}
		selectedFeatureIndex = index
		featureList.SetCurrentItem(index)
		refreshSubOptions()
	}
	featureList.SetChangedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		if index < 0 || index >= len(features) {
			index = 0
		}
		if selectedFeatureIndex == index {
			return
		}
		selectedFeatureIndex = index
		refreshSubOptions()
	})
	featureList.SetSelectedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		selectFeature(index)
		focusStartupList(app, featureList, subOptionList, subOptionList)
	})

	startAtSubOption := func(index int) {
		options := currentSubOptions()
		if len(options) == 0 {
			return
		}
		if index < 0 || index >= len(options) {
			index = 0
		}
		value := options[index].Value
		if strings.HasPrefix(value, "tool:") {
			result = StartupChoice{Tool: strings.TrimPrefix(value, "tool:")}
		} else {
			parts := strings.SplitN(value, ":", 2)
			if len(parts) == 2 {
				result = StartupChoice{
					Database: parts[0],
					Mode:     parts[1],
				}
			}
		}
		app.Stop()
	}
	start := func() {
		startAtSubOption(currentItem(subOptionList))
	}
	subOptionList.SetSelectedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		if index < 0 {
			index = 0
		}
		subOptionList.SetCurrentItem(index)
		focusStartupList(app, featureList, subOptionList, subOptionList)
	})

	root := startupRoot(app, info, featureList, subOptionList, start)
	setPageRoot(app, root)
	focusStartupList(app, featureList, subOptionList, featureList)
	switchNext := func() {
		if app.GetFocus() == featureList {
			focusStartupList(app, featureList, subOptionList, subOptionList)
		} else {
			focusStartupList(app, featureList, subOptionList, featureList)
		}
	}
	switchPrevious := func() {
		if app.GetFocus() == subOptionList {
			focusStartupList(app, featureList, subOptionList, featureList)
		} else {
			focusStartupList(app, featureList, subOptionList, subOptionList)
		}
	}
	installInputCapture(app, chainInputCaptures(remapTabCapture(switchNext, switchPrevious), func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEnter:
			if app.GetFocus() == featureList {
				focusStartupList(app, featureList, subOptionList, subOptionList)
				return nil
			}
			start()
			return nil
		case tcell.KeyRune:
			switch event.Rune() {
			case '1', '2', '3':
				index := int(event.Rune() - '1')
				if index >= 0 && index < len(features) {
					selectFeature(index)
					focusStartupList(app, featureList, subOptionList, featureList)
					return nil
				}
			case '4', '5', '6', '7', '8', '9':
				index := int(event.Rune() - '4')
				if index >= 0 && index < len(currentSubOptions()) {
					subOptionList.SetCurrentItem(index)
					focusStartupList(app, featureList, subOptionList, subOptionList)
					return nil
				}
			}
		}
		return event
	}))

	if err := runApp(app); err != nil {
		return StartupChoice{}, err
	}
	return result, nil
}

func newApp() *tview.Application {
	configStyles()
	app := tview.NewApplication().EnableMouse(true).EnablePaste(true)
	app.SetMouseCapture(func(event *tcell.EventMouse, action tview.MouseAction) (*tcell.EventMouse, tview.MouseAction) {
		if event == nil {
			return nil, action
		}
		x, y := event.Position()
		if x < 0 || y < 0 {
			app.DontDrawOnThisEventMouse()
			return nil, action
		}
		return event, action
	})
	return app
}

func runApp(app *tview.Application) (err error) {
	restoreConsoleCloseHandler := installDeferredAppHook(app, installConsoleCloseHandler)
	defer restoreConsoleCloseHandler()
	stopResizePoller := installDeferredAppHook(app, installConsoleResizeWatcher)
	defer stopResizePoller()
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("terminal UI crashed: %v\n%s", recovered, debug.Stack())
		}
	}()
	return app.Run()
}

func installDeferredAppHook(app *tview.Application, installer func(*tview.Application) func()) func() {
	if app == nil || installer == nil {
		return func() {}
	}

	var (
		mu        sync.Mutex
		restore   = func() {}
		installed bool
		once      sync.Once
	)
	afterDraw := app.GetAfterDrawFunc()
	app.SetAfterDrawFunc(func(screen tcell.Screen) {
		if afterDraw != nil {
			afterDraw(screen)
		}
		mu.Lock()
		defer mu.Unlock()
		if installed {
			return
		}
		restore = installer(app)
		installed = true
	})

	return func() {
		once.Do(func() {
			mu.Lock()
			restoreFunc := restore
			wasInstalled := installed
			mu.Unlock()
			if wasInstalled {
				restoreFunc()
			}
		})
	}
}

func configStyles() {
	tview.Styles = tview.Theme{
		PrimitiveBackgroundColor:    colorCanvas,
		ContrastBackgroundColor:     colorPanel,
		MoreContrastBackgroundColor: colorAccent,
		BorderColor:                 colorBorder,
		TitleColor:                  colorBorder,
		GraphicsColor:               colorBorder,
		PrimaryTextColor:            colorText,
		SecondaryTextColor:          colorMuted,
		TertiaryTextColor:           colorAccent,
		InverseTextColor:            colorAction,
		ContrastSecondaryTextColor:  colorInactiveText,
	}
}

type inputCaptureFunc func(*tcell.EventKey) *tcell.EventKey

func installInputCapture(app *tview.Application, handler inputCaptureFunc) {
	app.SetInputCapture(handler)
}

func chainInputCaptures(captures ...inputCaptureFunc) inputCaptureFunc {
	return func(event *tcell.EventKey) *tcell.EventKey {
		for _, capture := range captures {
			if capture == nil {
				continue
			}
			event = capture(event)
			if event == nil {
				return nil
			}
		}
		return event
	}
}

func remapTabCapture(next func(), previous func()) inputCaptureFunc {
	return func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab:
			next()
			return nil
		case tcell.KeyBacktab:
			previous()
			return nil
		}
		return event
	}
}

func startupRoot(app *tview.Application, info StartupInfo, featureList *tview.List, subOptionList *tview.List, start func()) tview.Primitive {
	description := "Select one function and one sub-option before continuing."

	infoView := tview.NewTextView().
		SetDynamicColors(true).
		SetTextColor(tview.Styles.PrimaryTextColor).
		SetText(fmt.Sprintf(
			"[white]%s helps you run keyword searches, BLAST workflows, and pathway-oriented tools across plant protein resources.\n\n[yellow]Author[white] %s\n[yellow]Repository[white] %s\n[yellow]License[white] %s%s",
			productName(info),
			fallbackText(info.Author, "unknown"),
			fallbackText(info.RepoURL, "unknown"),
			fallbackText(info.LicenseName, "unknown"),
			formatLicenseID(info.LicenseID),
		))

	module := newButtonFlex()
	module.AddItem(infoView, 6, 0, false)
	module.AddItem(tview.NewTextView().SetText(description).SetTextColor(tview.Styles.PrimaryTextColor), 2, 0, false)
	module.AddItem(featureList, 0, 1, true)
	module.AddItem(subOptionList, 0, 1, false)
	addButtonRow(module, buttonRow(
		buttonSpec{Label: ButtonStart, Shortcut: ShortcutConfirm, Action: func() {
			if app.GetFocus() == featureList {
				focusStartupList(app, featureList, subOptionList, subOptionList)
				return
			}
			start()
		}, Visible: true, Primary: true},
	))
	module.AddItem(hintView("Tab/Left/Right switch selection box | Up/Down choose item | 1-3 choose function | 4+ choose sub-option"), 1, 0, false)
	module.AddItem(hintView("Enter moves from function to sub-option, then starts the selected workflow."), 1, 0, false)

	moduleFrame := tview.NewFrame(module)
	moduleFrame.SetBorder(true)
	moduleFrame.SetTitle(" Startup ")
	moduleFrame.SetTitleAlign(tview.AlignCenter)

	return pageFrame(productName(info)+" > Startup", moduleFrame)
}

func pageFrame(breadcrumb string, body tview.Primitive) tview.Primitive {
	frame := tview.NewFrame(body)
	frame.SetBorders(0, 0, 0, 0, 0, 0)
	frame.AddText(elideBreadcrumb(breadcrumb, 96), true, tview.AlignLeft, tview.Styles.SecondaryTextColor)
	return frame
}

func formatLicenseID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	return " (" + id + ")"
}

type buttonSpec struct {
	Label    string
	Shortcut string
	Action   func()
	Visible  bool
	Primary  bool
}

func buttonRow(buttons ...buttonSpec) *buttonRowPrimitive {
	return newButtonRow(buttons...)
}

func optionList(title string, options []Option) *tview.List {
	return optionListWithStart(title, options, 1)
}

func optionListWithStart(title string, options []Option, start int) *tview.List {
	list := tview.NewList()
	setOptionListItems(list, title, options, start)
	list.SetBorder(true)
	list.SetTitleAlign(tview.AlignCenter)
	list.ShowSecondaryText(true)
	list.SetMainTextColor(tview.Styles.PrimaryTextColor)
	list.SetSecondaryTextColor(tview.Styles.SecondaryTextColor)
	list.SetSelectedTextColor(tview.Styles.InverseTextColor)
	list.SetSelectedBackgroundColor(tview.Styles.ContrastBackgroundColor)
	setFocusBorder(list.Box, false)
	attachFocusBorder(list.Box)
	return list
}

func setOptionListItems(list *tview.List, title string, options []Option, start int) {
	list.Clear()
	for i, option := range options {
		shortcut := rune('0' + start + i)
		if start+i > 9 {
			shortcut = 0
		}
		list.AddItem(option.Label, indentSecondary(option.Description), shortcut, nil)
	}
	list.SetTitle(" " + trimColon(title) + " ")
	if len(options) > 0 {
		list.SetCurrentItem(0)
	}
}

func focusStartupList(app *tview.Application, featureList *tview.List, subOptionList *tview.List, target *tview.List) {
	app.SetFocus(target)
}

func hintView(text string) *tview.TextView {
	return tview.NewTextView().
		SetText(text).
		SetTextColor(tview.Styles.SecondaryTextColor)
}

func currentItem(list *tview.List) int {
	index := list.GetCurrentItem()
	if index < 0 {
		return 0
	}
	return index
}

func moveChoiceSelection(list *tview.List, choices []Choice, delta int) {
	if list == nil || len(choices) == 0 {
		return
	}
	if delta == 0 {
		delta = 1
	}
	index := currentItem(list)
	for step := 0; step < len(choices); step++ {
		index = (index + delta + len(choices)) % len(choices)
		if strings.TrimSpace(choices[index].Value) != "" {
			list.SetCurrentItem(index)
			return
		}
	}
}

func productName(info StartupInfo) string {
	name := strings.TrimSpace(info.DisplayName)
	if name == "" {
		return "phytozome GO"
	}
	return name
}

func fallbackText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func trimColon(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), ":：")
}

func elideBreadcrumb(value string, maxWidth int) string {
	value = strings.TrimSpace(value)
	if maxWidth <= 0 || len([]rune(value)) <= maxWidth {
		return value
	}
	parts := strings.Split(value, ">")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	for len(parts) > 1 {
		candidate := "... > " + strings.Join(parts[1:], " > ")
		if len([]rune(candidate)) <= maxWidth {
			return candidate
		}
		parts = parts[1:]
	}
	runes := []rune(value)
	if maxWidth <= 3 {
		return string(runes[len(runes)-maxWidth:])
	}
	return "..." + string(runes[len(runes)-(maxWidth-3):])
}
