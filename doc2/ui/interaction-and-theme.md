# TUI Interaction and Theme Contract

## Text and color

- Default text, option labels, control labels, IDs, metadata, and shortcut tokens are white.
- Explanatory, secondary, inactive, and help prose is light gray.
- Focused borders and focused titles are pale blue. Text on pale-blue or pale-yellow selected/focused backgrounds is black, including primary buttons and highlighted rows/cells.
- Yellow is reserved for warnings and deliberate highlights. Canvas may keep its pale-yellow focus/highlight exception.
- Green and red retain semantic selected/on and unselected/off meanings.

Every screen has one bottom shortcut/help row. It never wraps. Fragments are joined in reading order, and overflow ends with `...`; narrow widths show only the available dots.

## Keyboard and mouse

- `Tab` moves between modules or fields inside the current screen or modal.
- A single-line input uses Enter to advance or apply.
- In a multi-field form, Enter advances until the final field; the final field applies. `Ctrl+R` may apply from any field.
- In a multiline input, Enter inserts a newline and `Ctrl+R` applies.
- Buttons, checkboxes, radio choices, and dropdowns activate with Space. Button rows can receive focus; arrows choose a button and Space activates it.
- On screens without text input, arrow keys navigate controls. On screens with text input, Left/Right remain owned by the input cursor; Up/Down navigate candidates or controls only where the page explicitly supports it.
- Mouse clicks focus the control they operate. Focus styling is identical whether focus came from mouse or keyboard.
- The homepage search grid keeps its purpose-built interaction model.

## Lifetime and rendering

- Pointer-only mouse motion must not redraw the full screen when no control has hover behavior and no drag is active.
- Resize events are coalesced. Windows viewport polling starts one worker per running application and joins it during cleanup.
- Search uses one debounced worker and a latest-request queue, rather than one goroutine per keystroke.
- Task/progress pages use one periodic UI refresh worker regardless of task update frequency.
- Page/overlay background work receives cancellation. Closing a page stops timers and rejects stale queued UI updates.
- Application queue producers must be released when the event loop stops, including initialization failure and saturated queues.
