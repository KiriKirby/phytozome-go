// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package tui

import (
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

var backgroundStore = struct {
	sync.Mutex
	current tview.Primitive
}{}

type backgroundCarrier interface {
	backgroundPrimitive() tview.Primitive
}

type rememberedBackgroundRoot struct {
	tview.Primitive
	background tview.Primitive
}

func (r *rememberedBackgroundRoot) backgroundPrimitive() tview.Primitive {
	if r == nil || r.background == nil {
		return nil
	}
	return r.background
}

func backgroundPrimitiveFor(root tview.Primitive) tview.Primitive {
	if carrier, ok := root.(backgroundCarrier); ok {
		if background := carrier.backgroundPrimitive(); background != nil {
			return background
		}
	}
	return root
}

func rememberRootBackground(root tview.Primitive, background tview.Primitive) tview.Primitive {
	if root == nil {
		return nil
	}
	if background == nil {
		background = root
	}
	return &rememberedBackgroundRoot{
		Primitive:  root,
		background: backgroundPrimitiveFor(background),
	}
}

func rememberBackground(root tview.Primitive) {
	if root == nil {
		return
	}
	root = backgroundPrimitiveFor(root)
	backgroundStore.Lock()
	backgroundStore.current = root
	backgroundStore.Unlock()
}

func currentBackground(fallback tview.Primitive) tview.Primitive {
	backgroundStore.Lock()
	current := backgroundStore.current
	backgroundStore.Unlock()
	if current != nil {
		return current
	}
	return fallback
}

func setPageRoot(app *tview.Application, root tview.Primitive) {
	rememberBackground(root)
	app.SetRoot(root, true)
}

type modalOverlay struct {
	*tview.Box
	child        tview.Primitive
	width        int
	height       int
	childX       int
	childY       int
	childWidth   int
	childHeight  int
	lastSetFocus func(p tview.Primitive)
}

func newModalOverlay(child tview.Primitive, width int, height int) *modalOverlay {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 16
	}
	return &modalOverlay{
		Box:    tview.NewBox(),
		child:  child,
		width:  width,
		height: height,
	}
}

func (m *modalOverlay) Draw(screen tcell.Screen) {
	x, y, w, h := m.GetRect()
	childW := m.width
	if childW > w-4 {
		childW = w - 4
	}
	if childW < 20 {
		childW = w
	}
	childH := m.height
	if childH > h-4 {
		childH = h - 4
	}
	if childH < 6 {
		childH = h
	}
	childX := x + (w-childW)/2
	childY := y + (h-childH)/2
	if childX < x {
		childX = x
	}
	if childY < y {
		childY = y
	}
	m.childX = childX
	m.childY = childY
	m.childWidth = childW
	m.childHeight = childH
	style := tcell.StyleDefault.Background(tview.Styles.PrimitiveBackgroundColor).Foreground(tview.Styles.PrimaryTextColor)
	for row := childY; row < childY+childH; row++ {
		for column := childX; column < childX+childW; column++ {
			screen.SetContent(column, row, ' ', nil, style)
		}
	}
	m.child.SetRect(childX, childY, childW, childH)
	m.child.Draw(screen)
}

func (m *modalOverlay) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return m.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if handler := m.child.InputHandler(); handler != nil {
			handler(event, setFocus)
		}
	})
}

func (m *modalOverlay) Focus(delegate func(p tview.Primitive)) {
	m.lastSetFocus = delegate
	if delegate != nil && m.child != nil {
		delegate(m.child)
		return
	}
	m.Box.Focus(delegate)
}

func (m *modalOverlay) HasFocus() bool {
	return m.Box.HasFocus() || (m.child != nil && m.child.HasFocus())
}

func (m *modalOverlay) Blur() {
	m.Box.Blur()
	if m.child != nil {
		m.child.Blur()
	}
}

func (m *modalOverlay) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
	return m.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
		if event == nil || !m.InRect(event.Position()) {
			return false, nil
		}
		x, y := event.Position()
		insideChild := x >= m.childX && x < m.childX+m.childWidth && y >= m.childY && y < m.childY+m.childHeight
		if insideChild {
			if handler := m.child.MouseHandler(); handler != nil {
				consumed, capture := handler(action, event, setFocus)
				if consumed {
					return true, capture
				}
			}
		}
		if setFocus != nil {
			setFocus(m)
		}
		return true, m
	})
}

func (m *modalOverlay) PasteHandler() func(text string, setFocus func(p tview.Primitive)) {
	return m.WrapPasteHandler(func(text string, setFocus func(p tview.Primitive)) {
		if handler := m.child.PasteHandler(); handler != nil {
			handler(text, setFocus)
		}
	})
}

func modalRoot(page TaskPage, body tview.Primitive, width int, height int) tview.Primitive {
	fallback := pageFrame(pageBreadcrumb(page.Breadcrumb, page.Path), tview.NewBox())
	background := currentBackground(fallback)
	root := tview.NewPages()
	root.AddPage("background", background, true, true)
	root.AddPage("modal", newModalOverlay(body, width, height), true, true)
	return rememberRootBackground(root, background)
}

func taskModalRoot(page TaskPage, body tview.Primitive, width int, height int) tview.Primitive {
	fallback := pageFrame(pageBreadcrumb(page.Breadcrumb, page.Path), tview.NewBox())
	background := currentBackground(fallback)
	root := tview.NewPages()
	root.AddPage("background", background, true, true)
	root.AddPage("modal", newModalOverlay(body, width, height), true, true)
	return rememberRootBackground(root, background)
}

func infoModalRoot(page InfoPage, body tview.Primitive, width int, height int) tview.Primitive {
	fallback := pageFrame(pageBreadcrumb(page.Breadcrumb, page.Path), tview.NewBox())
	background := currentBackground(fallback)
	root := tview.NewPages()
	root.AddPage("background", background, true, true)
	root.AddPage("modal", newModalOverlay(body, width, height), true, true)
	return rememberRootBackground(root, background)
}

func overlayRootOn(background tview.Primitive, modal tview.Primitive, width int, height int) tview.Primitive {
	if background == nil {
		background = currentBackground(tview.NewBox())
	}
	root := tview.NewPages()
	root.AddPage("background", background, true, true)
	root.AddPage("modal", newModalOverlay(modal, width, height), true, true)
	return rememberRootBackground(root, background)
}
