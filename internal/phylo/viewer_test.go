package phylo

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return data
}

func TestViewerServerStartsEmptyAndAcceptsPayload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := NewViewerServer("127.0.0.1:0")
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	base := server.URL()
	if base == "" {
		t.Fatal("server URL is empty")
	}
	resp, err := http.Get(base + "/health")
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d", resp.StatusCode)
	}
	resp, err = http.Get(base + "/sessions/test")
	if err != nil {
		t.Fatalf("session request failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if cache := resp.Header.Get("Cache-Control"); !strings.Contains(cache, "no-store") {
		t.Fatalf("session page Cache-Control = %q, want no-store", cache)
	}
	if !looksLikeViewerAppShell(string(body)) {
		t.Fatalf("initial page should serve the Reactree app shell: %s", body)
	}
	resp, err = http.Get(base + "/phgo-icon.png")
	if err != nil {
		t.Fatalf("icon request failed: %v", err)
	}
	iconBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || len(iconBody) == 0 {
		t.Fatalf("icon status/body = %d/%d, want embedded icon", resp.StatusCode, len(iconBody))
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "image/png") {
		t.Fatalf("icon Content-Type = %q, want image/png", resp.Header.Get("Content-Type"))
	}
	resp, err = http.Get(base + "/sessions/test/payload")
	if err != nil {
		t.Fatalf("initial payload request failed: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), `"session_id":"test"`) || !strings.Contains(string(body), `"newick":""`) {
		t.Fatalf("initial payload should be empty for the session: %s", body)
	}
	payload := []byte(`{"schema_version":1,"newick":"(PHGOT000001);","updated_at":"` + time.Now().Format(time.RFC3339Nano) + `"}`)
	req, err := http.NewRequest(http.MethodPut, base+"/sessions/test/payload", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("payload request failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("payload status = %d", resp.StatusCode)
	}
	resp, err = http.Get(base + "/sessions/test/payload")
	if err != nil {
		t.Fatalf("payload get failed: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), `(PHGOT000001);`) {
		t.Fatalf("payload should contain Newick after update: %s", body)
	}
}

func TestViewerAssetsEmbedReactreeApp(t *testing.T) {
	index, err := viewerAsset("index.html")
	if err != nil {
		t.Fatalf("viewer index asset missing: %v", err)
	}
	if !looksLikeViewerAppShell(string(index)) {
		t.Fatalf("viewer index does not look like the built Reactree app: %s", index)
	}
}

func TestViewerAssetsIncludeJalviewBootstrapPage(t *testing.T) {
	bootstrap, err := viewerAsset("jalview-bootstrap.html")
	if err != nil {
		t.Fatalf("jalview bootstrap asset missing: %v", err)
	}
	html := string(bootstrap)
	if !strings.Contains(html, `SwingJS.getApplet("JalviewJSEmbedded", window.Info)`) ||
		!strings.Contains(html, `jalview_EMBEDDED: true`) ||
		!strings.Contains(html, `/assets/jalviewjs/phgo-bridge.js`) ||
		!strings.Contains(html, `/assets/msaexpor/style.css`) ||
		!strings.Contains(html, `/assets/msaexpor/pdf.js`) ||
		!strings.Contains(html, `/assets/msaexpor/index.js`) ||
		!strings.Contains(html, `args: bridge.args`) ||
		!strings.Contains(html, `window.__PHGOJalviewInitialState = null`) ||
		!strings.Contains(html, `main: "jalview.bin.Jalview"`) ||
		!strings.Contains(html, `core: "jvexamplefile"`) ||
		!strings.Contains(html, `jalview-alignment-div`) ||
		!strings.Contains(html, `/assets/jalviewjs/swingjs/swingjs2.js`) ||
		!strings.Contains(html, `j2sPath: "/assets/jalviewjs/swingjs/j2s"`) ||
		!strings.Contains(html, `body.phgo-jalview-ready #jalview-desktop-div`) ||
		!strings.Contains(html, `#jalview-desktop-div.phgo-window-manager`) ||
		!strings.Contains(html, `display: block !important`) ||
		!strings.Contains(html, `visibility: visible !important`) ||
		!strings.Contains(html, `.phgo-root-desktop-frame`) ||
		!strings.Contains(html, `[id*="_DesktopPaneUI"]`) ||
		!strings.Contains(html, `#jalview-desktop-div .phgo-root-desktop-frame [id*="_MenuBarUI"]`) ||
		!strings.Contains(html, `#jalview-alignment-div [id*="_MenuBarUI"]`) ||
		!strings.Contains(html, `scrollbar-color: rgba(96, 94, 92, 0.62) rgba(243, 242, 241, 0.9)`) ||
		!strings.Contains(html, `*::-webkit-scrollbar-thumb`) ||
		!strings.Contains(html, `pointer-events: none`) ||
		strings.Contains(html, "body.phgo-jalview-ready #jalview-desktop-div,\r\n      #jalview-desktop-div.phgo-window-manager {\r\n        display: none !important") ||
		strings.Contains(html, "body.phgo-jalview-ready #jalview-desktop-div,\n      #jalview-desktop-div.phgo-window-manager {\n        display: none !important") ||
		strings.Contains(html, `aria-hidden="true"`) ||
		strings.Contains(html, `phgo-hidden-desktop`) ||
		strings.Contains(html, `.phgo-root-desktop-frame .ui-j2smenu,`) ||
		strings.Contains(html, `max-width: calc(100dvw - 8px)`) ||
		strings.Contains(html, `z-index: 2500 !important`) ||
		strings.Contains(html, `#jalview-alignment-div .swingjs-window`) {
		t.Fatalf("jalview bootstrap asset does not look like the local bootstrap page: %s", html)
	}
}

func TestViewerAssetsIncludePHgoJalviewBridge(t *testing.T) {
	bridge, err := viewerAsset("assets/jalviewjs/phgo-bridge.js")
	if err != nil {
		t.Fatalf("PHgo JalviewJS bridge asset missing: %v", err)
	}
	js := string(bridge)
	if !strings.Contains(js, `window.PHGOJalviewBridge`) ||
		!strings.Contains(js, `window.__PHGOJalviewInitialState`) ||
		!strings.Contains(js, "return `${window.location.origin}${target}`") ||
		!strings.Contains(js, `args: argsTarget ? ["open", argsTarget] : null`) ||
		!strings.Contains(js, `function adaptLayout()`) ||
		!strings.Contains(js, `phgo-jalview-ready`) ||
		!strings.Contains(js, `phgo-window-manager`) ||
		!strings.Contains(js, `desktop.removeAttribute("aria-hidden")`) ||
		!strings.Contains(js, `window.__PHGOJalviewState`) ||
		!strings.Contains(js, `function applyMainFrameMode(frame)`) ||
		!strings.Contains(js, `function hideMenusFromOutsideEvent(event)`) ||
		!strings.Contains(js, `function closeAllSwingMenus(reason)`) ||
		!strings.Contains(js, `j2smenu.collapseAll`) ||
		!strings.Contains(js, `dom-menu-close-secondary`) ||
		!strings.Contains(js, `node.style.removeProperty("visibility")`) ||
		!strings.Contains(js, `document.addEventListener("mousedown", hideMenusFromOutsideEvent, true)`) ||
		!strings.Contains(js, `document.addEventListener("keydown", hideMenusOnEscape, true)`) ||
		!strings.Contains(js, `document.addEventListener("pointerdown", hideMenusFromOutsideEvent, true)`) ||
		!strings.Contains(js, `desktop.phgoMainAlignmentFrame`) ||
		!strings.Contains(js, `window.__PHGOJalviewBridgeAPI`) ||
		!strings.Contains(js, `collectMSAState`) ||
		!strings.Contains(js, `collectMSASequences`) ||
		!strings.Contains(js, `collectMSAFeatures`) ||
		!strings.Contains(js, `state.features = collectMSAFeatures(frame)`) ||
		!strings.Contains(js, `state.colours = collectMSAColours(frame)`) ||
		!strings.Contains(js, `state.markers = collectMSAMarkers(frame)`) ||
		!strings.Contains(js, `javaMapToObject`) ||
		!strings.Contains(js, `const full = fullState !== false`) ||
		!strings.Contains(js, `scheduleMSAStateSave("selection-toggle", 250, false)`) ||
		!strings.Contains(js, `let msaStateSaveChain = Promise.resolve()`) ||
		!strings.Contains(js, `await saveTask`) ||
		!strings.Contains(js, `saveMSAStateNow`) ||
		!strings.Contains(js, `saveMSAStateManual`) ||
		!strings.Contains(js, `removePHgoRestoredAnnotations(frame)`) ||
		!strings.Contains(js, `removePHgoRestoredSequenceFeatures(frame)`) ||
		!strings.Contains(js, `removePHgoRestoredGroups(frame)`) ||
		!strings.Contains(js, `openMSAExportImageWindow`) ||
		!strings.Contains(js, `openMSAExportImageWindowSafe`) ||
		!strings.Contains(js, `MSA export window failed`) ||
		!strings.Contains(js, `createSwingChildWindow`) ||
		!strings.Contains(js, `attachPanelToFrame`) ||
		!strings.Contains(js, `renderMSAExportScene`) ||
		!strings.Contains(js, `java.awt.image.BufferedImage`) ||
		!strings.Contains(js, `data-msaexpor-renderer="jalview-vector"`) ||
		!strings.Contains(js, `installSwingColourSwatchFix`) ||
		!strings.Contains(js, `ensureMSAExportWindowMounted`) ||
		!strings.Contains(js, `const leftLabelPadding = Math.max(24, Math.ceil(charWidth * 3));`) ||
		!strings.Contains(js, `SwingJS child-window content pane is not available for msaexpor`) ||
		!strings.Contains(js, `frameDOMNodeByTitle`) ||
		!strings.Contains(js, `addFrameToJalviewDesktop`) ||
		!strings.Contains(js, `Jalview Desktop.addInternalFrame is required`) ||
		!strings.Contains(js, `function clazzClass(name)`) ||
		!strings.Contains(js, `window.Clazz._4Name(name, null, null, true)`) ||
		!strings.Contains(js, `getRootPane$`) ||
		!strings.Contains(js, `rootPane.setContentPane$java_awt_Container(panel)`) ||
		!strings.Contains(js, `getContentPane$`) ||
		!strings.Contains(js, `add$java_awt_Component$O(panel, "Center")`) ||
		!strings.Contains(js, `javax.swing.JInternalFrame`) ||
		!strings.Contains(js, `frameClass.c$$S$Z$Z$Z$Z, [title, true, true, true, true]`) ||
		!strings.Contains(js, `__PHGOMSAExportWindow`) ||
		!strings.Contains(js, `runtime.frameWindow.PHGOmsaexpor.renderApp`) ||
		!strings.Contains(js, `writeMSAExportIframeDocument`) ||
		!strings.Contains(js, `/assets/msaexpor/pdf.js`) ||
		!strings.Contains(js, `/assets/msaexpor/index.js`) ||
		!strings.Contains(js, `waitForMSAExportFrameRuntime`) ||
		!strings.Contains(js, `configureMSAExportFrameWindow`) ||
		!strings.Contains(js, `__PHGO_MSAEXPOR_PARENT_WINDOW__`) ||
		!strings.Contains(js, `displayPrefixForSequence`) ||
		!strings.Contains(js, `/msa/state`) ||
		!strings.Contains(js, `payloadUpdatedAt`) ||
		!strings.Contains(js, `currentPayloadUpdatedAt`) ||
		!strings.Contains(js, `setConservationColour$java_awt_Color`) ||
		!strings.Contains(js, `currentColourSchemeState`) ||
		!strings.Contains(js, `colours.scheme = scheme`) ||
		!strings.Contains(js, `normalizeSavedColourScheme`) ||
		!strings.Contains(js, `registeredColourScheme`) ||
		!strings.Contains(js, `userColourSchemeFromSaved`) ||
		!strings.Contains(js, `toAppletParameter$`) ||
		!strings.Contains(js, `jalview.schemes.UserColourScheme`) ||
		!strings.Contains(js, `colour_scheme_name`) ||
		!strings.Contains(js, `applySavedColourScheme`) ||
		!strings.Contains(js, `changeColour_actionPerformed$S`) ||
		!strings.Contains(js, `Reloading MSA...`) ||
		!strings.Contains(js, `checkboxColumnWidth`) ||
		strings.Contains(js, `installApplyMenuFallback`) ||
		strings.Contains(js, `desktop.setAttribute("aria-hidden", "true")`) ||
		strings.Contains(js, `node.style.visibility = "hidden"`) ||
		strings.Contains(js, `selectionStateForName`) ||
		strings.Contains(js, `toggleSelectionForName`) ||
		strings.Contains(js, `selectionEntryForSequenceName`) ||
		strings.Contains(js, `saveMSAStateManual(); applyMSASelection`) ||
		strings.Contains(js, `window.setInterval(updateMSACheckboxLayer`) ||
		strings.Contains(js, `phgo-hidden-desktop`) ||
		strings.Contains(js, `notify("layout-warning"`) ||
		strings.Contains(js, `function fixSwingMenuPositioning(reason)`) ||
		strings.Contains(js, `fixSwingMenuPositioning("startup")`) ||
		strings.Contains(js, `scheduleMSAStateSave("startup"`) ||
		strings.Contains(js, `scheduleMSAStateSave("layout-ready"`) ||
		strings.Contains(js, `setBackgroundColour$java_awt_Color`) ||
		strings.Contains(js, `setResidueShading$O`) ||
		strings.Contains(js, `setGlobalColourScheme$O`) ||
		strings.Contains(js, `["colour_scheme", ["getGlobalColourScheme$"]]`) ||
		strings.Contains(js, `["residue_shading", ["getResidueShading$"]]`) ||
		strings.Contains(js, `colours.colour_scheme ||`) ||
		strings.Contains(js, `settings.colour_scheme ||`) ||
		strings.Contains(js, `const components = menu && typeof menu.getMenuComponents$`) ||
		strings.Contains(js, `colourSchemeObjectByName`) ||
		!strings.Contains(js, `resizeMainAlignmentFrame`) ||
		!strings.Contains(js, `window.__PHGOJalviewScheduleResizeMainAlignment`) ||
		strings.Contains(js, `desktop.instance.setBounds$I$I$I$I`) ||
		strings.Contains(js, `relayoutComponentTree`) ||
		strings.Contains(js, `alignment.querySelectorAll(".swingjs-window")`) ||
		strings.Contains(js, `hideAlignmentDecorations(alignment)`) ||
		strings.Contains(js, `msaexpor-content-pane-fallback`) ||
		strings.Contains(js, `mounting into generated frame DOM`) ||
		strings.Contains(js, `window.PHGOmsaexpor.renderApp`) ||
		strings.Contains(js, `msaexpor asset is not loaded`) ||
		strings.Contains(js, `addFrameDirectlyToDesktop`) ||
		strings.Contains(js, `msaexpor-direct-desktop-add`) ||
		strings.Contains(js, `collectResidueStyleForSequence`) ||
		strings.Contains(js, `residue_colors`) ||
		strings.Contains(js, `collectMSAStyle`) ||
		strings.Contains(js, `leftLabelWidth = Math.min(420`) {
		t.Fatalf("PHgo JalviewJS bridge does not expose the expected PHgo integration contract: %s", js)
	}
}

func TestViewerAssetsIncludeMSAExporModule(t *testing.T) {
	js, err := viewerAsset("assets/msaexpor/index.js")
	if err != nil {
		t.Fatalf("msaexpor index asset missing: %v", err)
	}
	body := string(js)
	for _, want := range []string{
		`window.PHGOmsaexpor`,
		`parseAdvancedLayout`,
		`buildAutomaticLayout`,
		`validateModel`,
		`No MSA rows to export`,
		`No aligned MSA columns to export`,
		`renderScene`,
		`bridge.renderMSAExportScene(settings, layout)`,
		`MSA export requires the Jalview native render bridge`,
		`all-row shorthand must be the only item token`,
		`if (itemsText === "~")`,
		`parseAlignedFasta`,
		`const sourceRows = sequenceRows.length > 0 ? sequenceRows : metadataRows`,
		`generateScene({ scale: 1 })`,
		`Preview needs refresh`,
		`showSaveFilePicker`,
		`MSA export requires the PHgo save bridge or the browser save-file picker`,
		`finally`,
		`writable.close`,
		`canvas.toBlob`,
		`2D canvas context is unavailable`,
		`Generate`,
		`Cancel`,
		`Export canceled.`,
		`Export file written.`,
		`host.addEventListener(type, isolate)`,
		`host.innerHTML = ""`,
		`titleNumberFilenameBase`,
		`DEFAULT_SETTINGS`,
		`cellWidth: 9`,
		`cellHeight: 13`,
		`columnNumberInterval: 20`,
		`showRightResidueNumbers = true`,
		`PDF export requires bundled jspdf and svg2pdf.js assets`,
		`MSA export requires a prepared save target.`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("msaexpor index asset missing %q", want)
		}
	}
	for _, forbidden := range []string{
		`html2canvas`,
		`getDisplayMedia`,
		`toDataURL`,
		`querySelector(".selection")`,
		`fallbackToDownloadOnError`,
		`downloadURL`,
		`RESIDUE_COLORS`,
		`DEFAULT_STYLE`,
		`renderSVG`,
		`host.addEventListener(type, isolate, true)`,
		`target ? target.save`,
		`: saveBlob(`,
		`saveBlob,`,
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("msaexpor index asset contains forbidden screenshot/UI capture path %q", forbidden)
		}
	}

	css, err := viewerAsset("assets/msaexpor/style.css")
	if err != nil {
		t.Fatalf("msaexpor style asset missing: %v", err)
	}
	if !strings.Contains(string(css), `.msaexpor-app`) || !strings.Contains(string(css), `border-radius: 4px`) || !strings.Contains(string(css), `background-image:`) {
		t.Fatalf("msaexpor style asset does not contain the expected PHgo/MSA theme")
	}

	pdf, err := viewerAsset("assets/msaexpor/pdf.js")
	if err != nil {
		t.Fatalf("msaexpor PDF asset missing: %v", err)
	}
	pdfBody := string(pdf)
	if !strings.Contains(pdfBody, `PHGOmsaexporPDF`) ||
		!strings.Contains(pdfBody, `PDF export failed: generated SVG is not valid XML`) ||
		strings.Contains(pdfBody, `export{`) {
		t.Fatalf("msaexpor PDF asset is not a browser global bundle")
	}
}

func TestMSAExporGeneratePreparesSaveTargetBeforeRendering(t *testing.T) {
	js, err := viewerAsset("assets/msaexpor/index.js")
	if err != nil {
		t.Fatalf("msaexpor index asset missing: %v", err)
	}
	body := string(js)
	saveOffset := strings.Index(body, `const target = await prepareSaveTargetForSettings(state.settings, baseName);`)
	renderOffset := strings.Index(body, `const { scene } = generateScene();`)
	exportOffset := strings.Index(body, `await exportScene(scene, state.settings, baseName, target);`)
	for name, offset := range map[string]int{
		"save target preparation": saveOffset,
		"native scene generation": renderOffset,
		"targeted export":         exportOffset,
	} {
		if offset < 0 {
			t.Fatalf("msaexpor Generate handler missing %s", name)
		}
	}
	if !(saveOffset < renderOffset && renderOffset < exportOffset) {
		t.Fatalf("msaexpor Generate must prepare save target before native rendering and write to that target")
	}
	if strings.Contains(body, `<a download`) || strings.Contains(body, `.click()`) {
		t.Fatalf("msaexpor must not include an anchor-download save fallback")
	}
}

func TestMSAExporNativeVectorBridgeRenderingContract(t *testing.T) {
	bridge, err := viewerAsset("assets/jalviewjs/phgo-bridge.js")
	if err != nil {
		t.Fatalf("PHgo bridge asset missing: %v", err)
	}
	js := string(bridge)
	for _, want := range []string{
		`function renderMSAExportScene(settings, layout)`,
		`const exportSettings = normalizeMSAExportSettings(settings);`,
		`function rowLengthStatsFromText(value)`,
		`function buildMSAExportLabel(taxonID, name, index, sequenceValue, settings)`,
		`function exportLabelForSequence(taxonID, name, index, sequenceValue, settings)`,
		`const renderedBlocks = blocks.map((block) => ({ block, indexes: blockRowsToIndexes(block, alignmentHeight) }));`,
		`const label = exportRowLabel(seq, index, exportSettings);`,
		`let maxLabelTextWidth = 72;`,
		`const leftLabelPadding = Math.max(24, Math.ceil(charWidth * 3));`,
		`const leftLabelWidth = Math.ceil(Math.max(96, maxLabelTextWidth + leftLabelPadding));`,
		`const gridX = margin + leftLabelWidth;`,
		`const previousExportSettings = bridge.__msaexporRenderSettings;`,
		`const previousExportActive = window.__PHGO_MSAEXPOR_RENDER_ACTIVE__;`,
		`bridge.__msaexporRenderSettings = exportSettings;`,
		`window.__PHGO_MSAEXPOR_RENDER_ACTIVE__ = true;`,
		`function residueFormatStyle(viewport, alignment, seq, column)`,
		`function sequenceGroupAt(alignment, seq, column)`,
		`function residueShaderColour(shader, seq, column)`,
		`const format = residueFormatStyle(viewport, alignment, seq, col);`,
		`const fill = format.showBoxes ? residueCellColour(seqCanvas, alignPanel, viewport, seq, col, exportSettings) : "";`,
		`if (format.showBoxes && colorIsVisibleCellFill(fill)) {`,
		`if (format.showText && (renderGaps || (ch !== "-" && ch !== "."))) {`,
		`addSVGText(parts, ch, cellX + charWidth / 2, baseline, { anchor: "middle", fill: format.textFill, className: "msaexpor-residue" });`,
		`addGroupOutlines(parts, alignment, indexes, start, endExclusive, gridX, rowStartY, charWidth, charHeight);`,
		`bridge.__msaexporRenderSettings = previousExportSettings;`,
		`if (typeof previousExportActive !== "undefined") {`,
		`window.__PHGO_MSAEXPOR_RENDER_ACTIVE__ = previousExportActive;`,
		`delete window.__PHGO_MSAEXPOR_RENDER_ACTIVE__;`,
		`exportLabelForSequence,`,
		`data-msaexpor-renderer="jalview-vector"`,
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("PHgo bridge native renderer missing contract fragment %q", want)
		}
	}
	for _, forbidden := range []string{
		`leftLabelWidth = Math.min`,
		`drawAnnotation`,
		`drawWrappedPanelForPrinting`,
		`drawPanelForPrinting$java_awt_Graphics`,
		`printWrappedAlignment`,
		`makeAlignmentImage`,
		`drawScale$`,
		`html2canvas`,
		`getDisplayMedia`,
		`captureStream`,
		`querySelector(".selection")`,
		`collectResidueStyleForSequence`,
		`collectMSAStyle`,
		`residue_colors`,
		`canvas.toDataURL("image/png")`,
		`<image href=`,
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("PHgo bridge native renderer contains forbidden export path fragment %q", forbidden)
		}
	}
}

func TestMSAExporJavaScriptUnitTests(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not available")
	}
	cmd := exec.Command("node", "viewer_assets/assets/msaexpor/msaexpor.test.mjs")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("msaexpor JavaScript tests failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "msaexpor tests passed") {
		t.Fatalf("msaexpor JavaScript tests did not report success: %s", output)
	}
}

func TestPHGOJalviewBridgeFormatJavaScriptUnitTests(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not available")
	}
	cmd := exec.Command("node", "viewer_assets/assets/jalviewjs/phgo-bridge.test.mjs")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("PHgo bridge format JavaScript tests failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "phgo bridge format tests passed") {
		t.Fatalf("PHgo bridge format JavaScript tests did not report success: %s", output)
	}
}

func TestReactreeViewerJavaScriptUnitTests(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not available")
	}
	cmd := exec.Command("node", "../../tree-viewer/src/pgv.test.mjs")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Reactree viewer JavaScript tests failed: %v\n%s", err, output)
	}
}

func TestVendoredJalviewMSAExportImageRoutesToMSAExpor(t *testing.T) {
	for _, asset := range []string{
		"assets/jalviewjs/swingjs/j2s/jalview/jbgui/GAlignFrame.js",
		"assets/jalviewjs/swingjs/j2s/core/core_jalview.js",
		"assets/jalviewjs/swingjs/j2s/core/corejalview.js",
		"assets/jalviewjs/swingjs/j2s/core/corejvexamplefile.js",
	} {
		body, err := viewerAsset(asset)
		if err != nil {
			t.Fatalf("%s missing: %v", asset, err)
		}
		js := string(body)
		for _, want := range []string{
			`phgoExportImageMenuItem`,
			`Export image...`,
			`openMSAExportImageWindowSafe`,
			`fileMenu.add$javax_swing_JMenuItem(phgoExportImageMenuItem)`,
		} {
			if !strings.Contains(js, want) {
				t.Fatalf("%s missing msaexpor menu route %q", asset, want)
			}
		}
		if strings.Contains(js, `fileMenu.add$javax_swing_JMenuItem(exportImageMenu);`+"\n"+`fileMenu.add$javax_swing_JMenuItem(exportFeatures);`) {
			t.Fatalf("%s still adds the legacy export image submenu unconditionally", asset)
		}
	}
}

func TestVendoredSwingJSShowsTopLevelPopupMenuItems(t *testing.T) {
	body, err := viewerAsset("assets/jalviewjs/swingjs/j2s/swingjs/jquery/j2sMenu.js")
	if err != nil {
		t.Fatalf("vendored SwingJS j2sMenu asset missing: %v", err)
	}
	js := string(body)
	if !strings.Contains(js, `menu.$ulTop.children(".ui-j2smenu-item").css("display", "block");`) ||
		!strings.Contains(js, `menu.$ulTop.hide().j2smenu("noclickout").show();`) {
		t.Fatalf("SwingJS popup menu show path should restore direct top-level menu items before showing")
	}
}

func TestVendoredJalviewCoreHasPHgoSourcePatches(t *testing.T) {
	core, err := viewerAsset("assets/jalviewjs/swingjs/j2s/core/corejvexamplefile.js")
	if err != nil {
		t.Fatalf("vendored JalviewJS core missing: %v", err)
	}
	js := string(core)
	for _, want := range []string{
		`window.__PHGOJalviewState`,
		`frame.__phgoMainAlignmentFrame=true`,
		`phgoAlignment.nucleotide=phgoNucleotide`,
		`frame.buildColourMenu$()`,
		`window.__PHGOJalviewResizeMainAlignment=function()`,
		`f.setBounds$I$I$I$I(0, 0, width, height)`,
		`var wasFrozen=this._boundsFrozen`,
		`this.__phgoMainAlignmentFrame || this.__phgoRootDesktopFrame`,
		`phgoCheckboxColumnWidth$`,
		`phgoDisplayPrefix$`,
		`phgoDisplayName$`,
		`drawPHgoCheckbox$java_awt_Graphics2D`,
		`id=s.getName$()`,
		`var prefix=this.phgoDisplayPrefix$jalview_datamodel_SequenceI$I`,
		`var string=this.phgoDisplayName$jalview_datamodel_SequenceI`,
		`string=prefix + " " + string`,
		`if (this.fastPaint && this.image != null )`,
		`}this.fastPaint=false;`,
		`this.image == null  || oldHeight != this.imgHeight`,
		`toggleSelectionForSequence`,
		`var phgoSave=Clazz_new_($I$(2).c$$S,["Save"])`,
		`var phgoApply=Clazz_new_($I$(2).c$$S,["Apply"])`,
		`bridge.saveMSAStateManual`,
		`bridge.applyMSASelection`,
		`applyPHgoFluentScrollBarStyle$`,
		`__phgoFluentScrollBarStyle`,
		`#8a8886`,
		`#f3f2f1`,
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("vendored JalviewJS core is missing PHgo patch %q", want)
		}
	}
	for _, forbidden := range []string{
		`this.add$javax_swing_JMenuItem(this.pdbStructureDialog);`,
		`this.add$javax_swing_JMenuItem(this.rnaStructureMenu);`,
		`this.editMenu.add$javax_swing_JMenuItem(this.toggle);`,
		`label.spaces_converted_to_underscores`,
		`.replace$C$C(" ", this.b$['jalview.gui.PopupMenu'].ap.av.getGapCharacter$())`,
		`var string=s.getDisplayId$Z(this.av.getShowJVSuffix$());`,
		`var string=sequence.getDisplayId$Z(alignViewport.getShowJVSuffix$());`,
		`}if (this.av.isRightAlignIds$() && String(prefix).length == 0) {`,
		`if (alignViewport.isRightAlignIds$() && String(prefix).length == 0) {`,
		`this.c && this.c.__phgoRootDesktopFrame`,
		`bridge.toggleSelectionForSequence(seqForPhgo.phgoTaxonID || "", seqForPhgo.getName$(), pos.seqIndex);` + "\r\n" + `this.getIdCanvas$().repaint$();`,
		`bridge.toggleSelectionForSequence(seqForPhgo.phgoTaxonID || "", seqForPhgo.getName$(), pos.seqIndex);` + "\n" + `this.getIdCanvas$().repaint$();`,
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("vendored JalviewJS core still contains disabled PHgo MSA behavior %q", forbidden)
		}
	}
}

func TestVendoredJalviewPopupMenuRemovesVarna2DStructure(t *testing.T) {
	for _, asset := range []string{
		"assets/jalviewjs/swingjs/j2s/jalview/gui/PopupMenu.js",
		"assets/jalviewjs/swingjs/j2s/core/core_jalview.js",
		"assets/jalviewjs/swingjs/j2s/core/corejalview.js",
		"assets/jalviewjs/swingjs/j2s/core/corejvexamplefile.js",
		"assets/jalviewjs/swingjs/j2s/core/core_jalview.z.js",
		"assets/jalviewjs/swingjs/j2s/core/corejvexamplefile.z.js",
	} {
		body, err := viewerAsset(asset)
		if err != nil {
			t.Fatalf("%s missing: %v", asset, err)
		}
		js := string(body)
		if strings.Contains(js, `this.add$javax_swing_JMenuItem(this.rnaStructureMenu);`) {
			t.Fatalf("%s still adds the VARNA/RNA 2D structure menu to the right-click popup", asset)
		}
	}
}

func TestVendoredJalviewIdCanvasBundlesStayInSync(t *testing.T) {
	for _, asset := range []string{
		"assets/jalviewjs/swingjs/j2s/jalview/gui/IdCanvas.js",
		"assets/jalviewjs/swingjs/j2s/core/core_jalview.js",
		"assets/jalviewjs/swingjs/j2s/core/corejalview.js",
		"assets/jalviewjs/swingjs/j2s/core/corejvexamplefile.js",
	} {
		body, err := viewerAsset(asset)
		if err != nil {
			t.Fatalf("%s missing: %v", asset, err)
		}
		js := string(body)
		for _, want := range []string{
			`phgoCheckboxColumnWidth$`,
			`phgoIsMSAExportRenderActive$`,
			`__PHGO_MSAEXPOR_RENDER_ACTIVE__`,
			`phgoSelectionState$jalview_datamodel_SequenceI$I`,
			`phgoDisplayPrefix$jalview_datamodel_SequenceI$I`,
			`phgoDisplayName$jalview_datamodel_SequenceI`,
			`phgoResidueRatio$jalview_datamodel_SequenceI`,
			`phgoDisplayLabel$jalview_datamodel_SequenceI$I`,
			`bridge.exportLabelForSequence`,
			`return bridge.exportLabelForSequence(seq && seq.phgoTaxonID || "", exportName, index, exportText, settings);`,
			`var last=-1;`,
			`!(ch === "*" && i === last)`,
			`drawPHgoCheckbox$java_awt_Graphics2D`,
			`var checkboxWidth=this.phgoCheckboxColumnWidth$();`,
			`var string=this.phgoDisplayLabel$jalview_datamodel_SequenceI$I`,
			`getSequenceAsString$()`,
			`residues + "/" + total`,
			`if (this.phgoIsMSAExportRenderActive$()) return 0;`,
			`if (this.phgoIsMSAExportRenderActive$()) return;`,
			`if (!this.phgoIsMSAExportRenderActive$() && (this.searchResults != null ) && this.searchResults.contains$O(s) )`,
			`}if (!this.phgoIsMSAExportRenderActive$() && selection != null  && selection.contains$O(sequence) )`,
			`this.drawPHgoCheckbox$java_awt_Graphics2D$jalview_datamodel_SequenceI$I$I$I$I$java_awt_Color`,
			`getDisplayId$Z(false)`,
		} {
			if !strings.Contains(js, want) {
				t.Fatalf("%s is missing synchronized PHgo IdCanvas patch %q", asset, want)
			}
		}
		for _, forbidden := range []string{
			`var string=s.getDisplayId$Z(this.av.getShowJVSuffix$());`,
			`var string=sequence.getDisplayId$Z(alignViewport.getShowJVSuffix$());`,
			`if (this.av.isRightAlignIds$()) {`,
			`if (alignViewport.isRightAlignIds$()) {`,
			`if (bridge && bridge.__msaexporRenderSettings) return 0;`,
			`} else if (!(window.__PHGOJalviewBridgeAPI && window.__PHGOJalviewBridgeAPI.__msaexporRenderSettings)`,
			`if ((this.searchResults != null ) && this.searchResults.contains$O(s) )`,
			`}if (selection != null  && selection.contains$O(sequence) )`,
			`if (ch !== "-" && ch !== "." && ch !== " " && ch !== "\t" && ch !== "\r" && ch !== "\n" && ch !== "*")`,
		} {
			if strings.Contains(js, forbidden) {
				t.Fatalf("%s still contains stale Jalview IdCanvas behavior %q", asset, forbidden)
			}
		}
	}
}

func TestVendoredJalviewMSAExportNativeRendererSuppressesTransientUI(t *testing.T) {
	for _, asset := range []string{
		"assets/jalviewjs/swingjs/j2s/jalview/gui/SeqCanvas.js",
		"assets/jalviewjs/swingjs/j2s/core/core_jalview.js",
		"assets/jalviewjs/swingjs/j2s/core/corejalview.js",
		"assets/jalviewjs/swingjs/j2s/core/corejvexamplefile.js",
	} {
		body, err := viewerAsset(asset)
		if err != nil {
			t.Fatalf("%s missing: %v", asset, err)
		}
		js := string(body)
		for _, want := range []string{
			`phgoIsMSAExportRenderActive$`,
			`__PHGO_MSAEXPOR_RENDER_ACTIVE__`,
			`if (!this.phgoIsMSAExportRenderActive$()) {`,
			`p$1.drawSelectionGroup$java_awt_Graphics2D$I$I$I$I.apply`,
			`var exportSettings=bridge && bridge.__msaexporRenderSettings;`,
			`if (this.av.isShowSequenceFeatures$() && (!exportSettings || exportSettings.showFeatures !== false)) {`,
			`}if (!this.phgoIsMSAExportRenderActive$() && this.av.cursorMode) {`,
			`if (this.phgoIsMSAExportRenderActive$()) {`,
			`return;`,
			`var group=this.av.getSelectionGroup$();`,
			`var cursor_ypos=this.cursorY;`,
			`}if (!this.phgoIsMSAExportRenderActive$() && this.av.hasSearchResults$()) {`,
			`if ((!exportSettings || exportSettings.showGroups !== false) && this.av.getAlignment$().getGroups$().size$() > 0 ) {`,
			`if (this.phgoIsMSAExportRenderActive$() || !this.av.isShowAnnotation$()) {`,
			`if (!this.phgoIsMSAExportRenderActive$() && this.av.isShowAnnotation$()) {`,
		} {
			if !strings.Contains(js, want) {
				t.Fatalf("%s is missing native MSA export transient-UI suppression %q", asset, want)
			}
		}
		for _, forbidden := range []string{
			`}if (this.av.hasSearchResults$()) {`,
			`if ((!exportSettings || exportSettings.showGroups !== false) && (this.av.getSelectionGroup$() != null  || this.av.getAlignment$().getGroups$().size$() > 0 )) {`,
			`if (this.av.getSelectionGroup$() != null  || this.av.getAlignment$().getGroups$().size$() > 0 ) {` + "\n" + `this.drawGroupsBoundaries$java_awt_Graphics$I$I$I$I$I`,
		} {
			if strings.Contains(js, forbidden) {
				t.Fatalf("%s still allows transient UI in native MSA export path %q", asset, forbidden)
			}
		}
	}
}

func TestVendoredJalviewCheckboxClickUsesNativeRepaintPath(t *testing.T) {
	for _, asset := range []string{
		"assets/jalviewjs/swingjs/j2s/jalview/gui/IdPanel.js",
		"assets/jalviewjs/swingjs/j2s/core/corejvexamplefile.js",
	} {
		body, err := viewerAsset(asset)
		if err != nil {
			t.Fatalf("%s missing: %v", asset, err)
		}
		js := string(body)
		for _, want := range []string{
			`bridge.toggleSelectionForSequence(seqForPhgo.phgoTaxonID || "", seqForPhgo.getName$(), pos.seqIndex, false);`,
			`bridge.invalidateIdCanvas(this.getIdCanvas$());`,
			`this.getIdCanvas$().repaint$();`,
			`this.alignPanel.paintAlignment$Z$Z(false, false);`,
		} {
			if !strings.Contains(js, want) {
				t.Fatalf("%s is missing native checkbox repaint path %q", asset, want)
			}
		}
		if strings.Contains(js, `bridge.toggleSelectionForSequence(seqForPhgo.phgoTaxonID || "", seqForPhgo.getName$(), pos.seqIndex);`) {
			t.Fatalf("%s still lets the bridge repaint synchronously during checkbox click", asset)
		}
	}
}

func TestJalviewBootstrapDisablesAnnotationsByDefault(t *testing.T) {
	html, err := viewerAsset("jalview-bootstrap.html")
	if err != nil {
		t.Fatalf("jalview bootstrap asset missing: %v", err)
	}
	if !strings.Contains(string(html), `jalview_SHOW_ANNOTATIONS: false`) {
		t.Fatalf("jalview bootstrap should disable annotations by default for PHgo")
	}
}

func TestJalviewPayloadMetadataNormalizesPreviewMode(t *testing.T) {
	payload := ViewerPayload{
		SchemaVersion: 1,
		SessionID:     "test",
		Metadata: Metadata{
			ConversionTarget: ConversionTargetProtein,
			Records: []InputRecord{
				{TaxonID: "PHGOT000001", DisplayName: "protein-ish", SequenceKind: SequenceUnknown},
			},
		},
	}
	normalized := normalizeJalviewPayloadMetadata(payload)
	if normalized.Metadata.SequenceKind != SequenceProtein {
		t.Fatalf("sequence kind = %q, want protein", normalized.Metadata.SequenceKind)
	}
	if normalized.Metadata.ConversionTarget != ConversionTargetProtein {
		t.Fatalf("conversion target = %q, want protein", normalized.Metadata.ConversionTarget)
	}

	payload.Metadata.SequenceKind = SequenceNucleotide
	normalized = normalizeJalviewPayloadMetadata(payload)
	if normalized.Metadata.SequenceKind != SequenceProtein {
		t.Fatalf("conversion target should override stale sequence kind, got %q", normalized.Metadata.SequenceKind)
	}

	payload.Metadata.ConversionTarget = ""
	payload.Metadata.Records[0].SequenceKind = SequenceNucleotide
	normalized = normalizeJalviewPayloadMetadata(payload)
	if normalized.Metadata.SequenceKind != SequenceNucleotide || normalized.Metadata.ConversionTarget != ConversionTargetDNA {
		t.Fatalf("metadata fallback = kind %q target %q", normalized.Metadata.SequenceKind, normalized.Metadata.ConversionTarget)
	}
}

func TestViewerMSAApplyKeepsPayloadOrderAndState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := NewViewerServer("127.0.0.1:0")
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	payload := ViewerPayload{
		SchemaVersion: 1,
		SessionID:     "canvas",
		UpdatedAt:     time.Now(),
		AlignedFASTA:  ">b\nBBB\n>a\nAAA\n",
		Metadata: Metadata{Records: []InputRecord{
			{TaxonID: "b", DisplayName: "B", CanvasItem: "group", CanvasRow: 1},
			{TaxonID: "a", DisplayName: "A", CanvasItem: "group", CanvasRow: 0},
		}},
	}
	server.SetMSAPayload("canvas", payload)
	var got MSAApplyRequest
	server.SetMSAApplyHandler(func(ctx context.Context, sessionID string, req MSAApplyRequest) (MSAApplyResponse, error) {
		got = req
		return MSAApplyResponse{Accepted: true}, nil
	})
	resp, err := http.Post(server.URL()+"/sessions/canvas/msa/apply", "application/json", strings.NewReader(`{"rows":[{"taxon_id":"a","state":"yellow"},{"taxon_id":"b","state":"green"}]}`))
	if err != nil {
		t.Fatalf("MSA apply post: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("MSA apply status = %d", resp.StatusCode)
	}
	if len(got.Rows) != 2 || got.Rows[0].TaxonID != "b" || got.Rows[1].TaxonID != "a" {
		t.Fatalf("apply request order = %#v, want payload record order", got.Rows)
	}
	state := server.GetMSAState("canvas")
	if len(state.Rows) != 2 || state.Rows[0].TaxonID != "b" || state.Rows[1].State != "yellow" {
		t.Fatalf("MSA state mismatch after apply: %#v", state)
	}
	nextPayload := payload
	nextPayload.Metadata.Records = []InputRecord{{TaxonID: "a", DisplayName: "A", CanvasItem: "group", CanvasRow: 0}}
	putViewerPayload(t, server.URL()+"/sessions/canvas/payload", mustJSON(t, nextPayload))
	state = server.GetMSAState("canvas")
	if len(state.Rows) != 1 || state.Rows[0].TaxonID != "a" || state.Rows[0].State != "yellow" {
		t.Fatalf("MSA state should follow new payload while preserving matching row state: %#v", state)
	}
}

func TestViewerMSAApplyRejectsExcessiveRows(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := NewViewerServer("127.0.0.1:0")
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	server.SetMSAPayload("canvas", ViewerPayload{
		SchemaVersion: 1,
		SessionID:     "canvas",
		UpdatedAt:     time.Now(),
		AlignedFASTA:  ">a\nAAA\n",
		Metadata:      Metadata{Records: []InputRecord{{TaxonID: "a", DisplayName: "A"}}},
	})
	var b strings.Builder
	b.WriteString(`{"rows":[`)
	for i := 0; i < 10001; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"taxon_id":"a","state":"green","index":%d}`, i)
	}
	b.WriteString(`]}`)
	resp, err := http.Post(server.URL()+"/sessions/canvas/msa/apply", "application/json", strings.NewReader(b.String()))
	if err != nil {
		t.Fatalf("MSA apply post: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("MSA apply excessive rows status = %d", resp.StatusCode)
	}
}

func TestJalviewAlignedFASTAUsesRawDisplayNameAndSelectionKeepsCoordinatePrefix(t *testing.T) {
	payload := ViewerPayload{
		SchemaVersion: 1,
		SessionID:     "canvas",
		UpdatedAt:     time.Now(),
		AlignedFASTA:  ">PHGOT000001\nMPEPTIDE\n",
		Metadata: Metadata{Records: []InputRecord{{
			TaxonID:         "PHGOT000001",
			DisplayName:     "[1,2] Alpha name",
			BaseDisplayName: "Alpha name",
			DisplayPrefix:   "[1,2]",
			CanvasItemIndex: 0,
			CanvasRow:       1,
		}}},
	}
	fasta := jalviewAlignedFASTA(payload)
	rawName := base64.RawURLEncoding.EncodeToString([]byte("Alpha name"))
	prefixedName := base64.RawURLEncoding.EncodeToString([]byte("[1,2] Alpha name"))
	if !strings.Contains(fasta, "phgo_name64:"+rawName) {
		t.Fatalf("Jalview FASTA should encode raw display name, got: %s", fasta)
	}
	if strings.Contains(fasta, "phgo_name64:"+prefixedName) {
		t.Fatalf("Jalview FASTA must not encode coordinate-prefixed name: %s", fasta)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := NewViewerServer("127.0.0.1:0")
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	server.SetMSAPayload("canvas", payload)
	body := getViewerPayload(t, server.URL()+"/sessions/canvas/msa/selection")
	if !strings.Contains(body, `"display_name":"Alpha name"`) ||
		!strings.Contains(body, `"display_prefix":"[1,2]"`) ||
		!strings.Contains(body, `"display_label":"[1,2] Alpha name"`) ||
		!strings.Contains(body, `"canvas_item_index":1`) ||
		!strings.Contains(body, `"canvas_row":2`) {
		t.Fatalf("MSA selection should split raw display name and coordinate prefix: %s", body)
	}
}

func TestViewerServerMSAStateEndpointPreservesDurableJalviewState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := NewViewerServer("127.0.0.1:0")
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	server.SetMSAPayload("canvas", ViewerPayload{
		SchemaVersion: 1,
		SessionID:     "canvas",
		UpdatedAt:     time.Now(),
		AlignedFASTA:  ">b\nBBB\n>a\nAAA\n",
		Metadata: Metadata{Records: []InputRecord{
			{TaxonID: "b", DisplayName: "B"},
			{TaxonID: "a", DisplayName: "A"},
		}},
	})
	req, err := http.NewRequest(http.MethodPut, server.URL()+"/sessions/canvas/msa/state", strings.NewReader(`{
		"schema_version": 1,
		"rows": [
			{"taxon_id":"a","state":"yellow"},
			{"taxon_id":"b","state":"green"}
		],
		"sequences": [
			{"taxon_id":"a","display_name":"Alpha Renamed","description":"manual desc","sequence":"m p e p"}
		],
		"settings": {"show_annotations": false, "wrap_alignment": true},
		"colours": {"text": "#112233", "scheme": "Clustal"},
		"groups": [{"name": "manual", "start": 1, "end": 4, "outline_colour":"#123456"}],
		"annotations": [{"label": "quality", "visible": true}],
		"features": [{"taxon_id":"a","display_name":"Alpha Renamed","type":"domain","begin":1,"end":4,"feature_group":"manual","links":["https://example.test/f"],"other_details":{"ID":"feat1"}}],
		"markers": [{"name":"bookmark","column":2}]
	}`))
	if err != nil {
		t.Fatalf("build MSA state request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT MSA state failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT MSA state status = %d", resp.StatusCode)
	}
	state := server.GetMSAState("canvas")
	if len(state.Rows) != 2 || state.Rows[0].TaxonID != "b" || state.Rows[0].State != "green" || state.Rows[1].TaxonID != "a" || state.Rows[1].State != "yellow" {
		t.Fatalf("MSA state rows should follow payload order with posted states: %#v", state.Rows)
	}
	if got, ok := state.Settings["wrap_alignment"].(bool); !ok || !got {
		t.Fatalf("MSA settings were not preserved: %#v", state.Settings)
	}
	if state.Colours["text"] != "#112233" || state.Colours["scheme"] != "Clustal" {
		t.Fatalf("MSA colours were not preserved: %#v", state.Colours)
	}
	if len(state.Groups) != 1 || state.Groups[0]["name"] != "manual" {
		t.Fatalf("MSA groups were not preserved: %#v", state.Groups)
	}
	if len(state.Annotations) != 1 || state.Annotations[0]["label"] != "quality" {
		t.Fatalf("MSA annotations were not preserved: %#v", state.Annotations)
	}
	if len(state.Features) != 1 || state.Features[0]["type"] != "domain" || state.Features[0]["taxon_id"] != "a" {
		t.Fatalf("MSA features were not preserved: %#v", state.Features)
	}
	if len(state.Markers) != 1 || state.Markers[0]["name"] != "bookmark" {
		t.Fatalf("MSA markers were not preserved: %#v", state.Markers)
	}
	if len(state.Sequences) != 1 || state.Sequences[0].TaxonID != "a" || state.Sequences[0].DisplayName != "Alpha Renamed" || state.Sequences[0].Description != "manual desc" || state.Sequences[0].Sequence != "m p e p" {
		t.Fatalf("MSA sequence edits were not preserved: %#v", state.Sequences)
	}
	partialReq, err := http.NewRequest(http.MethodPut, server.URL()+"/sessions/canvas/msa/state", strings.NewReader(`{
		"schema_version": 2,
		"rows": [{"taxon_id":"a","state":"green"}]
	}`))
	if err != nil {
		t.Fatalf("build partial MSA state request: %v", err)
	}
	partialResp, err := http.DefaultClient.Do(partialReq)
	if err != nil {
		t.Fatalf("PUT partial MSA state failed: %v", err)
	}
	_ = partialResp.Body.Close()
	if partialResp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT partial MSA state status = %d", partialResp.StatusCode)
	}
	state = server.GetMSAState("canvas")
	if len(state.Features) != 1 || len(state.Groups) != 1 || len(state.Annotations) != 1 || len(state.Sequences) != 1 || len(state.Markers) != 1 || len(state.Colours) != 2 {
		t.Fatalf("partial MSA state update should preserve durable Jalview state: %#v", state)
	}
	body := getViewerPayload(t, server.URL()+"/sessions/canvas/msa/state")
	if !strings.Contains(body, `"show_annotations":false`) || !strings.Contains(body, `"state":"green"`) || !strings.Contains(body, `"description":"manual desc"`) || !strings.Contains(body, `"features"`) || !strings.Contains(body, `"colours"`) || !strings.Contains(body, `"markers"`) {
		t.Fatalf("MSA state endpoint body missing durable state: %s", body)
	}
	fasta := getViewerPayload(t, server.URL()+"/sessions/canvas/aligned.fasta")
	renamed := base64.RawURLEncoding.EncodeToString([]byte("Alpha Renamed"))
	if !strings.Contains(fasta, "phgo_name64:"+renamed) || !strings.Contains(fasta, "mpep") || strings.Contains(fasta, "\nAAA\n") {
		t.Fatalf("saved MSA sequence state should be restored through aligned FASTA, got: %s", fasta)
	}
}

func TestViewerAssetsInlinePDFExportBundle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := NewViewerServer("127.0.0.1:0")
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	resp, err := http.Get(server.URL() + "/assets/index.js")
	if err != nil {
		t.Fatalf("asset request failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("index bundle status = %d", resp.StatusCode)
	}
	if len(body) == 0 {
		t.Fatal("index bundle body is empty")
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "javascript") {
		t.Fatalf("index bundle Content-Type = %q, want javascript", resp.Header.Get("Content-Type"))
	}
	bundle := string(body)
	if !strings.Contains(bundle, "application/pdf") {
		t.Fatal("index bundle should contain vector PDF export support")
	}
	if strings.Contains(bundle, "svg2pdf.es.min.js") {
		t.Fatal("index bundle should not reference a runtime svg2pdf dynamic chunk")
	}
	if strings.Contains(bundle, "jspdf.es.min.js") {
		t.Fatal("index bundle should not reference a runtime jspdf dynamic chunk")
	}
}

func looksLikeViewerAppShell(html string) bool {
	return strings.Contains(html, "<title>Phgotreer</title>") &&
		strings.Contains(html, `href="/phgo-icon.png"`) &&
		strings.Contains(html, `/assets/`) &&
		strings.Contains(html, `id="root"`)
}

func TestViewerServerSSEUpdatesAfterPayloadAndPreviewChanges(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := NewViewerServer("127.0.0.1:0")
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	streamCtx, streamCancel := context.WithTimeout(ctx, 5*time.Second)
	defer streamCancel()
	req, err := http.NewRequestWithContext(streamCtx, http.MethodGet, server.URL()+"/events/test", nil)
	if err != nil {
		t.Fatalf("build event request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("event request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("event status = %d", resp.StatusCode)
	}
	reader := bufio.NewReader(resp.Body)
	initial := readViewerSSEUpdate(t, reader)
	if !strings.Contains(initial, `"session_id":"test"`) || !strings.Contains(initial, `"seq":0`) {
		t.Fatalf("initial event should describe empty session: %q", initial)
	}

	putViewerPayload(t, server.URL()+"/sessions/test/payload", []byte(`{"schema_version":1,"newick":"(PHGOT000001);","updated_at":"`+time.Now().Format(time.RFC3339Nano)+`"}`))
	payloadUpdate := readViewerSSEUpdate(t, reader)
	if !strings.Contains(payloadUpdate, `"session_id":"test"`) || !strings.Contains(payloadUpdate, `"seq":1`) {
		t.Fatalf("payload update event should increment seq: %q", payloadUpdate)
	}

	putViewerPayload(t, server.URL()+"/sessions/test/preview", []byte(`{"layout":"circular"}`))
	previewUpdate := readViewerSSEUpdate(t, reader)
	if !strings.Contains(previewUpdate, `"session_id":"test"`) || !strings.Contains(previewUpdate, `"seq":2`) {
		t.Fatalf("preview update event should increment seq: %q", previewUpdate)
	}
}

func TestViewerServerServesMSAPage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := NewViewerServer("127.0.0.1:0")
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	putViewerPayload(t, server.URL()+"/sessions/test-session/payload", []byte(`{
		"schema_version": 1,
		"session_id": "test-session",
		"title": "Mode Probe",
		"aligned_fasta": ">SEQ1\nMPEPTIDE\n",
		"metadata": {
			"schema_version": 1,
			"sequence_kind": "protein",
			"conversion_target": "dna"
		}
	}`))
	resp, err := http.Get(server.URL() + "/sessions/test-session/msa")
	if err != nil {
		t.Fatalf("msa page request failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("msa page status = %d", resp.StatusCode)
	}
	if cache := resp.Header.Get("Cache-Control"); !strings.Contains(cache, "no-store") {
		t.Fatalf("msa page Cache-Control = %q, want no-store", cache)
	}
	html := string(body)
	if !strings.Contains(html, `SwingJS.getApplet("JalviewJSEmbedded", window.Info)`) ||
		strings.Contains(html, `http-equiv="refresh"`) {
		t.Fatalf("msa page should directly serve Jalview bootstrap HTML: %s", html)
	}
	state := decodeInlineJalviewBootstrapState(t, html)
	if state["session"] != "test-session" ||
		state["open"] != "/sessions/test-session/aligned.fasta" ||
		state["title"] != "Phgomsar: Mode Probe" ||
		state["sequenceKind"] != "nucleotide" ||
		state["conversionTarget"] != "dna" {
		t.Fatalf("unexpected Jalview bootstrap state: %#v", state)
	}

	resp, err = http.Get(server.URL() + "/msa/test-session")
	if err != nil {
		t.Fatalf("legacy msa page request failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("legacy msa page status = %d", resp.StatusCode)
	}
}

func TestViewerServerMSASelectionDefaultsAndApply(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := NewViewerServer("127.0.0.1:0")
	var applied MSAApplyRequest
	server.SetMSAApplyHandler(func(ctx context.Context, sessionID string, req MSAApplyRequest) (MSAApplyResponse, error) {
		if sessionID != "test" {
			t.Fatalf("apply sessionID = %q, want test", sessionID)
		}
		applied = req
		return MSAApplyResponse{Accepted: true, Message: "ok"}, nil
	})
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	putViewerPayload(t, server.URL()+"/sessions/test/payload", []byte(`{
		"schema_version": 1,
		"session_id": "test",
		"aligned_fasta": ">PHGOT000001\nAAAA\n>PHGOT000002\nBBBB\n",
		"metadata": {
			"schema_version": 1,
			"records": [
				{"taxon_id": "PHGOT000001", "display_name": "alpha"},
				{"taxon_id": "PHGOT000002", "display_name": "beta"}
			]
		}
	}`))

	initial := getViewerPayload(t, server.URL()+"/sessions/test/msa/selection")
	if !strings.Contains(initial, `"taxon_id":"PHGOT000001"`) || !strings.Contains(initial, `"state":"green"`) {
		t.Fatalf("initial MSA selection should default rows to green: %s", initial)
	}

	resp, err := http.Post(server.URL()+"/sessions/test/msa/apply", "application/json", strings.NewReader(`{
		"rows": [
			{"name": "alpha", "index": 0, "state": "yellow"},
			{"index": 1, "state": "red"},
			{"taxon_id": "", "state": "green"},
			{"taxon_id": "ignored", "state": "not-a-state"}
		]
	}`))
	if err != nil {
		t.Fatalf("MSA apply request failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("MSA apply status = %d", resp.StatusCode)
	}
	if len(applied.Rows) != 2 {
		t.Fatalf("applied rows = %#v, want two cleaned rows", applied.Rows)
	}
	after := getViewerPayload(t, server.URL()+"/sessions/test/msa/selection")
	if !strings.Contains(after, `"taxon_id":"PHGOT000001"`) || !strings.Contains(after, `"state":"yellow"`) ||
		!strings.Contains(after, `"taxon_id":"PHGOT000002"`) || !strings.Contains(after, `"state":"red"`) {
		t.Fatalf("MSA selection did not persist applied states: %s", after)
	}
}

func TestViewerMSAPageIncludesPayloadVersionForLiveReload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := NewViewerServer("127.0.0.1:0")
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	updatedAt := time.Now().UTC().Truncate(time.Nanosecond)
	putViewerPayload(t, server.URL()+"/sessions/test/payload", mustJSON(t, ViewerPayload{
		SchemaVersion: 1,
		SessionID:     "test",
		UpdatedAt:     updatedAt,
		AlignedFASTA:  ">PHGOT000001\nAAAA\n",
		Metadata: Metadata{Records: []InputRecord{
			{TaxonID: "PHGOT000001", DisplayName: "A"},
		}},
	}))

	html := getViewerPayload(t, server.URL()+"/sessions/test/msa")
	state := decodeInlineJalviewBootstrapState(t, html)
	if got, want := state["payloadUpdatedAt"], updatedAt.Format(time.RFC3339Nano); got != want {
		t.Fatalf("payloadUpdatedAt = %q, want %q", got, want)
	}
}

func TestViewerServerUsesSharedTreePayloadForMSAAlignmentAndSelection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := NewViewerServer("127.0.0.1:0")
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	putViewerPayload(t, server.URL()+"/sessions/test/payload", []byte(`{
		"schema_version": 1,
		"session_id": "test",
		"newick": "(TREE_ONLY);",
		"aligned_fasta": ">TREE_ONLY\nAAAA\n",
		"metadata": {
			"schema_version": 1,
			"records": [
				{"taxon_id": "TREE_ONLY", "display_name": "tree-only"}
			]
		}
	}`))
	server.SetMSAPayload("test", ViewerPayload{
		SchemaVersion: 1,
		SessionID:     "test",
		AlignedFASTA:  ">MSA_ONLY\nBBBB\n",
		Metadata: Metadata{
			Records: []InputRecord{{TaxonID: "MSA_ONLY", DisplayName: "msa-only"}},
		},
	})
	treePayload := getViewerPayload(t, server.URL()+"/sessions/test/payload")
	if !strings.Contains(treePayload, "TREE_ONLY") || strings.Contains(treePayload, "MSA_ONLY") {
		t.Fatalf("tree payload should remain the tree-only payload: %s", treePayload)
	}
	msaFasta := getViewerPayload(t, server.URL()+"/sessions/test/aligned.fasta")
	if !strings.Contains(msaFasta, "VFJFRV9PTkxZ") || strings.Contains(msaFasta, "TVNBX09OTFk") {
		t.Fatalf("MSA aligned FASTA should use the shared tree payload: %s", msaFasta)
	}
	selection := getViewerPayload(t, server.URL()+"/sessions/test/msa/selection")
	if !strings.Contains(selection, `"taxon_id":"TREE_ONLY"`) || strings.Contains(selection, "MSA_ONLY") {
		t.Fatalf("MSA selection should use the shared tree payload: %s", selection)
	}
}

func TestViewerServerSessionStatusEndpoint(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := NewViewerServer("127.0.0.1:0")
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	server.SetSessionStatus("test", ViewerSessionStatus{Refreshing: true, Message: "Refreshing tree and MSA..."})
	body := getViewerPayload(t, server.URL()+"/sessions/test/status")
	if !strings.Contains(body, `"refreshing":true`) || !strings.Contains(body, `"message":"Refreshing tree and MSA..."`) {
		t.Fatalf("status body = %s", body)
	}
}

func decodeJalviewBootstrapState(location string) (map[string]string, error) {
	_, encoded, ok := strings.Cut(location, "#phgo=")
	if !ok {
		return nil, io.ErrUnexpectedEOF
	}
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	var state map[string]string
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return state, nil
}

func decodeInlineJalviewBootstrapState(t *testing.T, html string) map[string]string {
	t.Helper()
	const marker = `window.__PHGOJalviewInitialState = `
	start := strings.Index(html, marker)
	if start < 0 {
		t.Fatalf("inline Jalview state marker missing from HTML: %s", html)
	}
	start += len(marker)
	end := strings.Index(html[start:], `;`)
	if end < 0 {
		t.Fatalf("inline Jalview state terminator missing from HTML: %s", html[start:])
	}
	var state map[string]string
	if err := json.Unmarshal([]byte(html[start:start+end]), &state); err != nil {
		t.Fatalf("decode inline Jalview state: %v\n%s", err, html[start:start+end])
	}
	return state
}

func TestViewerServerServesTreeSessionRoute(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := NewViewerServer("127.0.0.1:0")
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	putViewerPayload(t, server.URL()+"/sessions/test/payload", []byte(`{"schema_version":1,"session_id":"test","title":"Tree Probe","newick":"(A);","updated_at":"`+time.Now().Format(time.RFC3339Nano)+`"}`))
	resp, err := http.Get(server.URL() + "/sessions/test/tree")
	if err != nil {
		t.Fatalf("tree session route request failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tree session route status = %d", resp.StatusCode)
	}
	html := string(body)
	if !looksLikeViewerAppShell(html) {
		t.Fatalf("tree session route did not render viewer index: %s", html)
	}
}

func TestViewerServerServesJalviewBootstrapPage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := NewViewerServer("127.0.0.1:0")
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	resp, err := http.Get(server.URL() + "/jalview-bootstrap.html")
	if err != nil {
		t.Fatalf("bootstrap page request failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bootstrap page status = %d", resp.StatusCode)
	}
	html := string(body)
	if !strings.Contains(html, `SwingJS.getApplet("JalviewJSEmbedded", window.Info)`) || !strings.Contains(html, `jalview-alignment-div`) {
		t.Fatalf("bootstrap page body did not render JalviewJS startup script: %s", html)
	}

	resp, err = http.Get(server.URL() + "/jalview-bootstrap/test-stamp.html")
	if err != nil {
		t.Fatalf("query-free bootstrap path request failed: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("query-free bootstrap path status = %d", resp.StatusCode)
	}
	html = string(body)
	if !strings.Contains(html, `SwingJS.getApplet("JalviewJSEmbedded", window.Info)`) || !strings.Contains(html, `jalview-alignment-div`) {
		t.Fatalf("query-free bootstrap path did not render JalviewJS startup script: %s", html)
	}

	resp, err = http.Get(server.URL() + "/jalview-bootstrap/test-stamp.html?j2sv=bad")
	if err != nil {
		t.Fatalf("bootstrap query rejection request failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bootstrap path with query status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestViewerServerServesAlignedFASTA(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := NewViewerServer("127.0.0.1:0")
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	putViewerPayload(t, server.URL()+"/sessions/test/payload", []byte("{\"schema_version\":1,\"session_id\":\"test\",\"aligned_fasta\":\">SEQ1\\nMPEPTIDE\\n\"}"))
	resp, err := http.Get(server.URL() + "/sessions/test/aligned.fasta")
	if err != nil {
		t.Fatalf("aligned FASTA request failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("aligned FASTA status = %d", resp.StatusCode)
	}
	if got := strings.TrimSpace(string(body)); got != ">SEQ1\nMPEPTIDE" {
		t.Fatalf("aligned FASTA body = %q", got)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/plain") {
		t.Fatalf("aligned FASTA Content-Type = %q, want text/plain", resp.Header.Get("Content-Type"))
	}
}

func TestViewerServerServesJalviewAlignedFASTAWithPHgoDisplayNames(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := NewViewerServer("127.0.0.1:0")
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	payload := `{
		"schema_version": 1,
		"session_id": "test",
		"aligned_fasta": ">PHGOT000001 runtime desc\nMPEP-TIDE\n>PHGOT000002\nATG-C\n",
		"metadata": {
			"schema_version": 1,
			"records": [
				{"taxon_id": "PHGOT000001", "display_name": "Sp9509d012g_#2a /1-536 \u4e2d\u6587 (alpha)"},
				{"taxon_id": "PHGOT000002", "display_name": "beta name/with/slash#2"}
			]
		}
	}`
	putViewerPayload(t, server.URL()+"/sessions/test/payload", []byte(payload))
	resp, err := http.Get(server.URL() + "/sessions/test/aligned.fasta")
	if err != nil {
		t.Fatalf("aligned FASTA request failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("aligned FASTA status = %d", resp.StatusCode)
	}
	got := string(body)
	if !strings.Contains(got, ">phgo_name64:") || !strings.Contains(got, "phgo_taxon_id64:") {
		t.Fatalf("aligned FASTA did not use PHgo Jalview headers: %s", got)
	}
	if strings.Contains(got, ">PHGOT000001") || strings.Contains(got, "Sp9509d012g_#2a /1-536") {
		t.Fatalf("aligned FASTA leaked runtime/raw display header instead of encoded Jalview header: %s", got)
	}
	if !strings.Contains(got, "MPEP-TIDE\n>") || !strings.Contains(got, "ATG-C") {
		t.Fatalf("aligned FASTA sequence content changed: %s", got)
	}
}

func TestViewerServerPreviewEndpointMergesState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := NewViewerServer("127.0.0.1:0")
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	preview := getViewerPreview(t, server.URL()+"/sessions/test/preview")
	if len(preview) != 0 {
		t.Fatalf("initial preview should be empty: %#v", preview)
	}
	putViewerPayload(t, server.URL()+"/sessions/test/preview", []byte(`{"layout":"circular","show_alignment":true}`))
	putViewerPayload(t, server.URL()+"/sessions/test/preview", []byte(`{"show_alignment":false,"theme":"tree-yellow"}`))
	preview = getViewerPreview(t, server.URL()+"/sessions/test/preview")
	if preview["layout"] != "circular" {
		t.Fatalf("preview layout should be preserved after merge: %#v", preview)
	}
	if preview["show_alignment"] != false {
		t.Fatalf("preview show_alignment should be overwritten by latest value: %#v", preview)
	}
	if preview["theme"] != "tree-yellow" {
		t.Fatalf("preview theme should be stored: %#v", preview)
	}
}

func TestViewerServerStateEndpointRoundTripsWithoutSSEBroadcast(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := NewViewerServer("127.0.0.1:0")
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	stateURL := server.URL() + "/sessions/test/state"
	initial := getViewerState(t, stateURL)
	if strings.TrimSpace(initial) != "{}" {
		t.Fatalf("initial state = %s", initial)
	}
	putViewerPayload(t, stateURL, []byte(`{"schema_version":1,"reactree":{"layout":"circular"}}`))
	got := getViewerState(t, stateURL)
	if !strings.Contains(got, `"layout":"circular"`) {
		t.Fatalf("viewer state did not round-trip: %s", got)
	}
}

func TestViewerServerKeepsPayloadAndStatePerSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := NewViewerServer("127.0.0.1:0")
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	putViewerPayload(t, server.URL()+"/sessions/canvas/payload", []byte(`{"schema_version":1,"newick":"(CANVAS);","updated_at":"`+time.Now().Format(time.RFC3339Nano)+`"}`))
	putViewerPayload(t, server.URL()+"/sessions/nwk-browser-1/payload", []byte(`{"schema_version":1,"newick":"(BROWSER);","updated_at":"`+time.Now().Format(time.RFC3339Nano)+`"}`))
	putViewerPayload(t, server.URL()+"/sessions/canvas/state", []byte(`{"schema_version":1,"reactree":{"layout":"rectangular"}}`))
	putViewerPayload(t, server.URL()+"/sessions/nwk-browser-1/state", []byte(`{"schema_version":1,"reactree":{"layout":"circular"}}`))

	canvasPayload := getViewerPayload(t, server.URL()+"/sessions/canvas/payload")
	browserPayload := getViewerPayload(t, server.URL()+"/sessions/nwk-browser-1/payload")
	if !strings.Contains(canvasPayload, "(CANVAS);") || strings.Contains(canvasPayload, "(BROWSER);") {
		t.Fatalf("canvas payload leaked browser state: %s", canvasPayload)
	}
	if !strings.Contains(browserPayload, "(BROWSER);") || strings.Contains(browserPayload, "(CANVAS);") {
		t.Fatalf("browser payload leaked canvas state: %s", browserPayload)
	}
	if got := getViewerState(t, server.URL()+"/sessions/canvas/state"); !strings.Contains(got, `"layout":"rectangular"`) || strings.Contains(got, "circular") {
		t.Fatalf("canvas state leaked browser state: %s", got)
	}
	if got := getViewerState(t, server.URL()+"/sessions/nwk-browser-1/state"); !strings.Contains(got, `"layout":"circular"`) || strings.Contains(got, "rectangular") {
		t.Fatalf("browser state leaked canvas state: %s", got)
	}
}

func readViewerSSEUpdate(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	var lines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE event: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if len(lines) > 0 {
				return strings.Join(lines, "\n")
			}
			continue
		}
		lines = append(lines, line)
	}
}

func putViewerPayload(t *testing.T, url string, payload []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build PUT request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT request failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT status = %d", resp.StatusCode)
	}
}

func getViewerPreview(t *testing.T, url string) map[string]any {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("preview request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preview status = %d", resp.StatusCode)
	}
	var preview map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&preview); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	return preview
}

func getViewerState(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("state request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("state status = %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	return string(body)
}

func getViewerPayload(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("payload request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("payload status = %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read payload: %v", err)
	}
	return string(body)
}
