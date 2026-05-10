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

func rememberBackground(root tview.Primitive) {
	if root == nil {
		return
	}
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

func (m *modalOverlay) SetSize(width int, height int) {
	if m == nil {
		return
	}
	if width > 0 {
		m.width = width
	}
	if height > 0 {
		m.height = height
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
	root := tview.NewPages()
	root.AddPage("background", currentBackground(fallback), true, true)
	root.AddPage("modal", newModalOverlay(body, width, height), true, true)
	return root
}

func taskModalRoot(page TaskPage, body tview.Primitive, width int, height int) tview.Primitive {
	root, _ := newTaskModalRoot(page, body, width, height)
	return root
}

func newTaskModalRoot(page TaskPage, body tview.Primitive, width int, height int) (tview.Primitive, *modalOverlay) {
	fallback := pageFrame(pageBreadcrumb(page.Breadcrumb, page.Path), tview.NewBox())
	root := tview.NewPages()
	root.AddPage("background", currentBackground(fallback), true, true)
	overlay := newModalOverlay(body, width, height)
	root.AddPage("modal", overlay, true, true)
	return root, overlay
}

func infoModalRoot(page InfoPage, body tview.Primitive, width int, height int) tview.Primitive {
	fallback := pageFrame(pageBreadcrumb(page.Breadcrumb, page.Path), tview.NewBox())
	root := tview.NewPages()
	root.AddPage("background", currentBackground(fallback), true, true)
	root.AddPage("modal", newModalOverlay(body, width, height), true, true)
	return root
}

func overlayRootOn(background tview.Primitive, modal tview.Primitive, width int, height int) tview.Primitive {
	if background == nil {
		background = currentBackground(tview.NewBox())
	}
	root := tview.NewPages()
	root.AddPage("background", background, true, true)
	root.AddPage("modal", newModalOverlay(modal, width, height), true, true)
	return root
}
