(function() {
  "use strict";

  const DEFAULT_SETTINGS = Object.freeze({
    format: "svg",
    scale: 2,
    cellWidth: 9,
    cellHeight: 13,
    showPHgoCoordinates: false,
    showLengthRatio: false,
    showLengthPercent: false,
    showAlignmentColumnNumbers: true,
    columnNumberInterval: 20,
    showRightResidueNumbers: false,
    showGroups: true,
    showFeatures: true,
    useAdvancedLayoutScript: false,
    advancedLayoutScript: ""
  });

  function normalizeSettings(settings) {
    const out = Object.assign({}, DEFAULT_SETTINGS, settings || {});
    out.format = String(out.format || "svg").toLowerCase();
    if (!["svg", "png", "pdf"].includes(out.format)) out.format = "svg";
    out.scale = [1, 2, 5, 10].includes(Number(out.scale)) ? Number(out.scale) : 2;
    out.cellWidth = positiveNumber(out.cellWidth, DEFAULT_SETTINGS.cellWidth);
    out.cellHeight = positiveNumber(out.cellHeight, DEFAULT_SETTINGS.cellHeight);
    out.columnNumberInterval = positiveInteger(out.columnNumberInterval, DEFAULT_SETTINGS.columnNumberInterval);
    for (const key of [
      "showPHgoCoordinates",
      "showLengthRatio",
      "showLengthPercent",
      "showAlignmentColumnNumbers",
      "showRightResidueNumbers",
      "showGroups",
      "showFeatures",
      "useAdvancedLayoutScript"
    ]) {
      out[key] = !!out[key];
    }
    if (out.useAdvancedLayoutScript) out.showRightResidueNumbers = true;
    out.advancedLayoutScript = String(out.advancedLayoutScript || "");
    return out;
  }

  function positiveNumber(value, defaultValue) {
    const n = Number(value);
    return Number.isFinite(n) && n > 0 ? n : defaultValue;
  }

  function positiveInteger(value, defaultValue) {
    const n = Number(value);
    return Number.isInteger(n) && n > 0 ? n : defaultValue;
  }

  function lineError(lineNumber, lineText, token, message) {
    const error = new Error(`Line ${lineNumber}: ${message}`);
    error.lineNumber = lineNumber;
    error.lineText = lineText;
    error.token = token;
    return error;
  }

  function parseCoordinate(token, lineNumber, lineText) {
    if (!/^[1-9][0-9]*,[1-9][0-9]*$/.test(token)) {
      throw lineError(lineNumber, lineText, token, `invalid PHgo row coordinate "${token}".`);
    }
    const [item, row] = token.split(",").map((part) => Number.parseInt(part, 10));
    return { item, row, key: `${item},${row}` };
  }

  function parseDisplayPrefixCoordinate(value) {
    const match = /^\[([1-9][0-9]*),([1-9][0-9]*)\]$/.exec(String(value || "").trim());
    if (!match) return null;
    return { item: Number.parseInt(match[1], 10), row: Number.parseInt(match[2], 10) };
  }

  function parseRangeEndpoint(token, alignmentWidth, lineNumber, lineText) {
    if (token === "~") return null;
    if (!/^(0|[1-9][0-9]*)$/.test(token)) {
      throw lineError(lineNumber, lineText, token, `invalid range endpoint "${token}".`);
    }
    const value = Number.parseInt(token, 10);
    if (value < 0 || value > alignmentWidth) {
      throw lineError(lineNumber, lineText, token, `range endpoint ${value} is outside [0, ${alignmentWidth}].`);
    }
    return value;
  }

  function resolveRange(rangeText, alignmentWidth, lineNumber, lineText) {
    const parts = rangeText.split(",");
    if (parts.length !== 2) {
      throw lineError(lineNumber, lineText, rangeText, "range must be written as start,end.");
    }
    const rawStart = parts[0];
    const rawEnd = parts[1];
    if (rawStart === "" || rawEnd === "") {
      throw lineError(lineNumber, lineText, rangeText, "range endpoints must not be empty.");
    }
    let start = parseRangeEndpoint(rawStart, alignmentWidth, lineNumber, lineText);
    let end = parseRangeEndpoint(rawEnd, alignmentWidth, lineNumber, lineText);
    if (start == null) start = 0;
    if (end == null) end = alignmentWidth;
    if (start >= end) {
      throw lineError(lineNumber, lineText, rangeText, "range start must be less than range end.");
    }
    return { start, end, length: end - start };
  }

  function parseAllocation(allocationText, total, lineNumber, lineText) {
    if (!allocationText) return [total];
    const tokens = allocationText.split(",");
    if (tokens.some((token) => token === "")) {
      throw lineError(lineNumber, lineText, allocationText, "allocation counts must not be empty.");
    }
    let fixed = 0;
    let placeholderCount = 0;
    const parsed = tokens.map((token) => {
      if (token === "~") {
        placeholderCount += 1;
        return "~";
      }
      if (!/^[1-9][0-9]*$/.test(token)) {
        throw lineError(lineNumber, lineText, token, `invalid allocation count "${token}".`);
      }
      const n = Number.parseInt(token, 10);
      fixed += n;
      return n;
    });
    if (fixed > total) {
      throw lineError(lineNumber, lineText, allocationText, `fixed allocation ${fixed} exceeds range length ${total}.`);
    }
    if (placeholderCount === 0) {
      if (fixed !== total) {
        throw lineError(lineNumber, lineText, allocationText, `allocation total ${fixed} must equal range length ${total}.`);
      }
      return parsed;
    }
    const remaining = total - fixed;
    if (remaining <= 0) {
      throw lineError(lineNumber, lineText, allocationText, "allocation placeholders have no remaining columns.");
    }
    if (remaining % placeholderCount !== 0) {
      throw lineError(lineNumber, lineText, allocationText, `remaining ${remaining} columns cannot be divided by ${placeholderCount} placeholders.`);
    }
    const fill = remaining / placeholderCount;
    return parsed.map((value) => value === "~" ? fill : value);
  }

  function normalizeRows(rows) {
    const out = [];
    const byCoordinate = new Map();
    const byTaxon = new Map();
    const byName = new Map();
    rows.forEach((row, index) => {
      const normalized = Object.assign({}, row);
      normalized.index = Number.isFinite(Number(row.index)) ? Number(row.index) : index;
      normalized.taxon_id = String(row.taxon_id || row.taxonID || "").trim();
      normalized.display_name = String(row.display_name || row.displayName || row.name || "").trim();
      normalized.display_prefix = String(row.display_prefix || row.displayPrefix || "").trim();
      normalized.display_label = String(row.display_label || row.displayLabel || "").trim();
      normalized.sequence = String(row.sequence || "");
      normalized.canvas_item_index = Number.parseInt(row.canvas_item_index ?? row.canvasItemIndex, 10);
      normalized.canvas_row = Number.parseInt(row.canvas_row ?? row.canvasRow, 10);
      // The DSL is a user-facing, one-based coordinate language.  Canvas keeps
      // its indexes zero-based internally, so a record can legitimately carry
      // both 0,0 machine coordinates and the visible [1,1] PHgo prefix.  The
      // prefix is the canonical public coordinate whenever it is present; only
      // falling back when numeric fields are absent made the first row (and every
      // other zero-based coordinate) impossible to select from a valid script.
      const coordinate = parseDisplayPrefixCoordinate(normalized.display_prefix);
      if (coordinate) {
        normalized.canvas_item_index = coordinate.item;
        normalized.canvas_row = coordinate.row;
      }
      normalized.state = String(row.state || "green").toLowerCase();
      out.push(normalized);
      if (Number.isFinite(normalized.canvas_item_index) && Number.isFinite(normalized.canvas_row)) {
        byCoordinate.set(`${normalized.canvas_item_index},${normalized.canvas_row}`, normalized);
      }
      if (normalized.taxon_id) byTaxon.set(normalized.taxon_id, normalized);
      if (normalized.display_name) byName.set(normalized.display_name, normalized);
    });
    return { rows: out, byCoordinate, byTaxon, byName };
  }

  function parseAdvancedLayout(script, model) {
    const alignmentWidth = model.alignmentWidth;
    const index = normalizeRows(model.rows || []);
    const blocks = [];
    const lines = String(script || "").split(/\r?\n/);
    for (let lineIndex = 0; lineIndex < lines.length; lineIndex += 1) {
      const lineNumber = lineIndex + 1;
      const rawLine = lines[lineIndex];
      const line = rawLine.trim();
      if (!line) continue;
      if (line.startsWith("#") || line.startsWith("//")) {
        throw lineError(lineNumber, rawLine, line, "comments are not supported.");
      }
      if (line.includes(" ") || line.includes("\t")) {
        throw lineError(lineNumber, rawLine, line, "internal spaces are not allowed.");
      }
      if (!line.startsWith(">")) {
        throw lineError(lineNumber, rawLine, line[0] || "", "expected line to start with \">\".");
      }
      const slashless = line.slice(1);
      const separators = slashless.split("\\");
      if (separators.length !== 2) {
        throw lineError(lineNumber, rawLine, "\\", "expected exactly one row/range separator \"\\\".");
      }
      const [itemsText, rangeAndAllocation] = separators;
      if (!itemsText) {
        throw lineError(lineNumber, rawLine, itemsText, "row item list must not be empty.");
      }
      const allocationParts = rangeAndAllocation.split("/");
      if (allocationParts.length > 2) {
        throw lineError(lineNumber, rawLine, rangeAndAllocation, "expected at most one allocation separator after the range.");
      }
      const rangeText = allocationParts[0];
      const allocationText = allocationParts[1] || "";
      let rows;
      if (itemsText === "~") {
        rows = index.rows;
      } else {
        if (itemsText.includes("~")) {
          throw lineError(lineNumber, rawLine, "~", "the all-row shorthand must be the only item token.");
        }
        const seen = new Set();
        rows = itemsText.split("/").map((token) => {
          const coordinate = parseCoordinate(token, lineNumber, rawLine);
          if (seen.has(coordinate.key)) {
            throw lineError(lineNumber, rawLine, token, `duplicate PHgo row coordinate ${token}.`);
          }
          seen.add(coordinate.key);
          const row = index.byCoordinate.get(coordinate.key);
          if (!row) {
            throw lineError(lineNumber, rawLine, token, `PHgo row coordinate ${token} was not found in the current MSA payload.`);
          }
          return row;
        });
      }
      const range = resolveRange(rangeText, alignmentWidth, lineNumber, rawLine);
      const counts = parseAllocation(allocationText, range.length, lineNumber, rawLine);
      const alignmentWidthForNumbering = Math.max(...counts);
      let cursor = range.start;
      counts.forEach((count, allocationIndex) => {
        blocks.push({
          sourceLine: lineNumber,
          allocationIndex,
          rows,
          columnStartBoundary: cursor,
          columnEndBoundary: cursor + count,
          visibleColumnCount: count,
          alignmentWidthForNumbering
        });
        cursor += count;
      });
    }
    if (blocks.length === 0) {
      throw new Error("Advanced layout script did not produce any export blocks.");
    }
    return { blocks };
  }

  function buildAutomaticLayout(model) {
    const rows = normalizeRows(model.rows || []).rows;
    const width = model.alignmentWidth;
    const blocks = width > 0 ? [{
      sourceLine: 0,
      allocationIndex: 0,
      rows,
      columnStartBoundary: 0,
      columnEndBoundary: width,
      visibleColumnCount: width,
      alignmentWidthForNumbering: width
    }] : [];
    return { blocks };
  }

  function resolveLayout(settings, model) {
    validateModel(model);
    return settings.useAdvancedLayoutScript
      ? parseAdvancedLayout(settings.advancedLayoutScript, model)
      : buildAutomaticLayout(model);
  }

  function validateModel(model) {
    const rows = Array.isArray(model && model.rows) ? model.rows : [];
    if (rows.length === 0) throw new Error("No MSA rows to export.");
    if (!Number.isFinite(Number(model.alignmentWidth)) || Number(model.alignmentWidth) <= 0) {
      throw new Error("No aligned MSA columns to export.");
    }
  }

  function renderScene(model, settings, layout, bridge) {
    if (bridge && typeof bridge.renderMSAExportScene === "function") {
      return bridge.renderMSAExportScene(settings, layout);
    }
    throw new Error("MSA export requires the Jalview native render bridge.");
  }

  async function loadModel(session, bridge) {
    const [payload, selection, savedState] = await Promise.all([
      fetchJSON(`/sessions/${encodeURIComponent(session)}/payload`),
      fetchJSON(`/sessions/${encodeURIComponent(session)}/msa/selection`),
      fetchJSON(`/sessions/${encodeURIComponent(session)}/msa/state`).catch(() => ({}))
    ]);
    let liveState = null;
    if (bridge && typeof bridge.collectMSAState === "function") {
      liveState = bridge.collectMSAState("msaexpor-open", { full: true });
    }
    const state = liveState || savedState || {};
    const rows = buildRows(payload, selection, state);
    const alignmentWidth = Math.max(0, ...rows.map((row) => String(row.sequence || "").length));
    return {
      session,
      payload,
      selection,
      state,
      rows,
      groups: Array.isArray(state.groups) ? state.groups : [],
      features: Array.isArray(state.features) ? state.features : [],
      alignmentWidth
    };
  }

  async function fetchJSON(path) {
    const response = await fetch(path, { cache: "no-store" });
    if (!response.ok) throw new Error(`${path} failed: ${response.status}`);
    return response.json();
  }

  function buildRows(payload, selection, state) {
    const sequenceRows = Array.isArray(state.sequences) ? state.sequences : parseAlignedFasta(payload && payload.aligned_fasta);
    const selectionRows = Array.isArray(selection && selection.rows) ? selection.rows : [];
    const metadataRows = payload && payload.metadata && Array.isArray(payload.metadata.records) ? payload.metadata.records : [];
    const sourceRows = sequenceRows.length > 0 ? sequenceRows : metadataRows;
    const sequenceByTaxon = mapRowsBy(sequenceRows, (row) => row.taxon_id || row.taxonID);
    const sequenceByName = mapRowsBy(sequenceRows, (row) => row.display_name || row.displayName || row.name);
    const metadataByTaxon = mapRowsBy(metadataRows, (row) => row.taxon_id || row.TaxonID);
    const metadataByName = mapRowsBy(metadataRows, (row) => row.display_name || row.displayName || row.name || row.base_display_name);
    const selectionByTaxon = mapRowsBy(selectionRows, (row) => row.taxon_id || row.taxonID);
    const selectionByName = mapRowsBy(selectionRows, (row) => row.display_name || row.displayName || row.name);
    return sourceRows.map((record, index) => {
      const taxonID = String(record.taxon_id || record.TaxonID || "").trim();
      const recordName = String(record.display_name || record.displayName || record.name || "").trim();
      const meta = metadataByTaxon.get(taxonID) || metadataByName.get(recordName) || metadataRows[index] || {};
      const sel = selectionByTaxon.get(taxonID) || selectionByName.get(recordName) || selectionRows[index] || {};
      const seq = sequenceByTaxon.get(taxonID) || sequenceByName.get(String(sel.display_name || sel.displayName || recordName).trim()) || sequenceRows[index] || record || {};
      return {
        taxon_id: taxonID || String(seq.taxon_id || "").trim(),
        display_name: String(sel.display_name || meta.base_display_name || meta.display_name || record.base_display_name || record.display_name || seq.display_name || seq.name || taxonID || "").trim(),
        display_prefix: String(sel.display_prefix || meta.display_prefix || record.display_prefix || "").trim(),
        display_label: String(sel.display_label || "").trim(),
        canvas_item_index: Number.parseInt(sel.canvas_item_index ?? meta.canvas_item_index ?? record.canvas_item_index, 10),
        canvas_row: Number.parseInt(sel.canvas_row ?? meta.canvas_row ?? record.canvas_row, 10),
        index,
        state: String(sel.state || "green").toLowerCase(),
        sequence: String(seq.sequence || record.sequence || "")
      };
    }).filter((row) => row.taxon_id || row.display_name || row.sequence);
  }

  function mapRowsBy(rows, selector) {
    const map = new Map();
    for (const row of rows || []) {
      const key = String(selector(row) || "").trim();
      if (key && !map.has(key)) map.set(key, row);
    }
    return map;
  }

  function parseAlignedFasta(fasta) {
    const rows = [];
    let current = null;
    for (const line of String(fasta || "").split(/\r?\n/)) {
      if (line.startsWith(">")) {
        current = { display_name: line.slice(1).trim().split(/\s+/)[0] || "", sequence: "" };
        rows.push(current);
      } else if (current) {
        current.sequence += line.trim();
      }
    }
    return rows;
  }

  function renderApp(host, options) {
    const bridge = options && options.bridge;
    const session = options && options.session;
    const runtimeWindow = host && host.ownerDocument && host.ownerDocument.defaultView ? host.ownerDocument.defaultView : window;
    const parentWindow = options && options.parentWindow
      ? options.parentWindow
      : (runtimeWindow.__PHGO_MSAEXPOR_PARENT_WINDOW__ || (runtimeWindow.parent && runtimeWindow.parent !== runtimeWindow ? runtimeWindow.parent : runtimeWindow));
    host.innerHTML = appShell();
    const state = { model: null, settings: normalizeSettings(), lastScene: null, error: "", previewDirty: false };
    const form = host.querySelector("form");
    const preview = host.querySelector("[data-role='preview']");
    const status = host.querySelector("[data-role='status']");
    const dirty = host.querySelector("[data-role='dirty']");
    const refresh = host.querySelector("[data-action='refresh-preview']");
    const exportButton = host.querySelector("[data-action='export']");
    const advanced = host.querySelector("[name='advancedLayoutScript']");
    for (const [key, value] of Object.entries(state.settings)) {
      const field = form.elements[key];
      if (!field) continue;
      if (field.type === "checkbox") field.checked = !!value;
      else field.value = value;
    }
    function syncFieldsFromSettings() {
      for (const [key, value] of Object.entries(state.settings)) {
        const field = form.elements[key];
        if (!field) continue;
        if (field.type === "checkbox") field.checked = !!value;
        else if (String(field.value) !== String(value)) field.value = value;
      }
      advanced.disabled = !state.settings.useAdvancedLayoutScript;
      const interval = form.elements.columnNumberInterval;
      if (interval) interval.disabled = !state.settings.showAlignmentColumnNumbers;
      const rightNumbers = form.elements.showRightResidueNumbers;
      if (rightNumbers) rightNumbers.disabled = state.settings.useAdvancedLayoutScript;
      const scale = form.elements.scale;
      if (scale) {
        scale.disabled = state.settings.format !== "png";
        scale.title = state.settings.format === "png" ? "PNG raster scale" : "Scale is only used for PNG export";
      }
    }
    function readSettings() {
      const data = {};
      for (const element of Array.from(form.elements)) {
        if (!element.name) continue;
        data[element.name] = element.type === "checkbox" ? element.checked : element.value;
      }
      state.settings = normalizeSettings(data);
      syncFieldsFromSettings();
    }
    function setDirty(value) {
      state.previewDirty = !!value;
      if (dirty) {
        dirty.textContent = state.previewDirty ? "Preview needs refresh" : "";
        dirty.hidden = !state.previewDirty;
      }
      if (refresh) refresh.disabled = !state.model;
    }
    function markPreviewDirty() {
      readSettings();
      setDirty(true);
    }
    function generateScene(overrides) {
      readSettings();
      if (!state.model) throw new Error("MSA export data is still loading.");
      const renderSettings = overrides ? normalizeSettings(Object.assign({}, state.settings, overrides)) : state.settings;
      const layout = resolveLayout(renderSettings, state.model);
      const scene = renderScene(state.model, renderSettings, layout, bridge);
      return { layout, scene };
    }
    function refreshPreview() {
      try {
        const { layout, scene } = generateScene({ scale: 1 });
        state.lastScene = scene;
        preview.innerHTML = scene.svg;
        const renderer = scene.source ? `, ${scene.source}` : "";
        status.textContent = `${scene.width} x ${scene.height} logical px preview, ${layout.blocks.length} block(s)${renderer}`;
        status.dataset.kind = "ok";
        setDirty(false);
      } catch (error) {
        state.lastScene = null;
        preview.innerHTML = "";
        status.textContent = error.message || String(error);
        status.dataset.kind = "error";
        setDirty(true);
      }
    }
    syncFieldsFromSettings();
    installControlEventIsolation(host);
    form.addEventListener("input", markPreviewDirty);
    form.addEventListener("change", markPreviewDirty);
    if (refresh) refresh.addEventListener("click", refreshPreview);
    exportButton.addEventListener("click", async () => {
      if (exportButton.disabled) return;
      exportButton.disabled = true;
      status.textContent = "Choose export location...";
      status.dataset.kind = "ok";
      try {
        readSettings();
        if (!state.model) throw new Error("MSA export data is still loading.");
        const title = parentWindow && parentWindow.document ? parentWindow.document.title : document.title;
        const baseName = defaultExportBaseName(state.model && state.model.payload, title, session || "msa");
        const target = await prepareSaveTargetForSettings(state.settings, baseName);
        if (target === false) {
          status.textContent = "Export canceled.";
          status.dataset.kind = "ok";
          return;
        }
        status.textContent = "Generating export...";
        const { scene } = generateScene();
        state.lastScene = scene;
        await exportScene(scene, state.settings, baseName, target);
        status.textContent = "Export file written.";
        status.dataset.kind = "ok";
      } catch (error) {
        status.textContent = error.message || String(error);
        status.dataset.kind = "error";
      } finally {
        exportButton.disabled = false;
      }
    });
    host.querySelector("[data-action='cancel']").addEventListener("click", () => {
      const win = parentWindow && parentWindow.__PHGOMSAExportWindow ? parentWindow.__PHGOMSAExportWindow : (window.__PHGOMSAExportWindow || {});
      if (win.frame && typeof win.frame.dispose$ === "function") {
        try { win.frame.dispose$(); } catch (_error) {}
      }
      if (win.hostParent && win.hostParent.parentNode) {
        win.hostParent.innerHTML = "";
      } else {
        host.innerHTML = "";
      }
      if (win.__watchTimer) {
        try { parentWindow.clearInterval(win.__watchTimer); } catch (_error) {}
      }
      if (win.__watchObserver && typeof win.__watchObserver.disconnect === "function") {
        try { win.__watchObserver.disconnect(); } catch (_error) {}
      }
      if (parentWindow) delete parentWindow.__PHGOMSAExportWindow;
      delete window.__PHGOMSAExportWindow;
    });
    loadModel(session, bridge).then((model) => {
      state.model = model;
      status.textContent = "Ready. Refresh preview to render the current settings.";
      status.dataset.kind = "ok";
      setDirty(true);
    }).catch((error) => {
      status.textContent = error.message || String(error);
      status.dataset.kind = "error";
      setDirty(true);
    });
  }

  function installControlEventIsolation(host) {
    const events = ["pointerdown", "mousedown", "mouseup", "click", "dblclick", "keydown", "keyup", "keypress", "wheel", "touchstart", "touchend"];
    const controlSelector = "input, select, textarea, button, option, label";
    for (const type of events) {
      const isolate = (event) => {
        const target = event.target;
        if (target && target.closest && target.closest(controlSelector)) {
          event.stopPropagation();
        }
      };
      host.addEventListener(type, isolate);
    }
  }

  function appShell() {
    return `
      <div class="msaexpor-app">
        <form class="msaexpor-settings">
          <section class="msaexpor-primary">
            <label>Format<select name="format"><option value="svg">SVG</option><option value="png">PNG</option><option value="pdf">PDF</option></select></label>
            <label>Scale<select name="scale"><option value="1">1x</option><option value="2">2x</option><option value="5">5x</option><option value="10">10x</option></select></label>
            <label>Cell width<input name="cellWidth" type="number" min="1" step="0.5"></label>
            <label>Cell height<input name="cellHeight" type="number" min="1" step="0.5"></label>
            <label>Column interval<input name="columnNumberInterval" type="number" min="1" step="1"></label>
          </section>
          <section class="msaexpor-checks" aria-label="Export content">
            <label><input name="showPHgoCoordinates" type="checkbox"> Show PHgo coordinates</label>
            <label><input name="showLengthRatio" type="checkbox"> Show length ratio</label>
            <label><input name="showLengthPercent" type="checkbox"> Show length percent</label>
            <label><input name="showAlignmentColumnNumbers" type="checkbox"> Show alignment column numbers</label>
            <label><input name="showRightResidueNumbers" type="checkbox"> Show right residue numbers</label>
            <label><input name="showGroups" type="checkbox"> Show groups</label>
            <label><input name="showFeatures" type="checkbox"> Show features</label>
            <label><input name="useAdvancedLayoutScript" type="checkbox"> Use advanced layout script</label>
          </section>
          <label class="msaexpor-script">Advanced layout script <span>Use rows/range\\blocks, for example &gt;1,4/1,3\\10,100/~,~,~</span><textarea name="advancedLayoutScript" spellcheck="false" placeholder=">1,4/1,3\\10,100/~,~,~"></textarea></label>
        </form>
        <div class="msaexpor-actions"><button type="button" data-action="refresh-preview">Refresh preview</button><span data-role="dirty" hidden></span><button type="button" data-action="export">Generate</button><button type="button" data-action="cancel">Cancel</button><span data-role="status"></span></div>
        <div class="msaexpor-preview" data-role="preview"></div>
      </div>`;
  }

  async function exportScene(scene, settings, basename, target) {
    if (!target || typeof target.save !== "function") {
      throw new Error("MSA export requires a prepared save target.");
    }
    if (settings.format === "svg") {
      const blob = new Blob([scene.svg], { type: "image/svg+xml;charset=utf-8" });
      return target.save(blob);
    }
    if (settings.format === "png") {
      const png = await svgToPNGBlob(scene.svg, scene.width, scene.height, settings.scale);
      return target.save(png);
    }
    if (settings.format === "pdf") {
      if (!window.PHGOmsaexporPDF || typeof window.PHGOmsaexporPDF.exportPDFBlob !== "function") {
        throw new Error("PDF export requires bundled jspdf and svg2pdf.js assets; SVG and PNG export are available.");
      }
      const blob = await window.PHGOmsaexporPDF.exportPDFBlob({
        svgString: scene.svg,
        width: scene.width,
        height: scene.height
      });
      return target.save(blob);
    }
    throw new Error(`Unsupported export format: ${settings.format}`);
  }

  function saveOptionsForSettings(settings, basename) {
    if (settings.format === "svg") {
      return {
        filename: `${basename}.svg`,
        description: "SVG image",
        accept: { "image/svg+xml": [".svg"] }
      };
    }
    if (settings.format === "png") {
      return {
        filename: `${basename}.png`,
        description: "PNG image",
        accept: { "image/png": [".png"] }
      };
    }
    if (settings.format === "pdf") {
      return {
        filename: `${basename}.pdf`,
        description: "PDF document",
        accept: { "application/pdf": [".pdf"] }
      };
    }
    throw new Error(`Unsupported export format: ${settings.format}`);
  }

  async function prepareSaveTargetForSettings(settings, basename) {
    const options = saveOptionsForSettings(settings, basename);
    return prepareSaveTarget(options.filename, options);
  }

  async function prepareSaveTarget(filename, options) {
    options = Object.assign({}, options || {});
    const safeName = sanitizeFilename(filename, "msa-export");
    const bridge = window.__PHGO_SAVE_BLOB__;
    if (typeof bridge === "function") {
      return {
        filename: safeName,
        save(blob) {
          return bridge(blob, safeName, options);
        }
      };
    }
    if (typeof window.showSaveFilePicker === "function" && window.isSecureContext) {
      try {
        const handle = await window.showSaveFilePicker({
          suggestedName: safeName,
          types: options.accept ? [{ description: options.description || "Exported file", accept: options.accept }] : []
        });
        return {
          filename: safeName,
          async save(blob) {
            let writable = null;
            try {
              writable = await handle.createWritable();
              await writable.write(blob);
              await writable.close();
              writable = null;
              return true;
            } finally {
              if (writable && typeof writable.close === "function") {
                try { await writable.close(); } catch (_error) {}
              }
            }
          }
        };
      } catch (error) {
        if (error && error.name === "AbortError") return false;
        throw error;
      }
    }
    throw new Error("MSA export requires the PHgo save bridge or the browser save-file picker.");
  }

  async function saveBlob(filename, blob, options) {
    const target = await prepareSaveTarget(filename, options);
    if (target === false) return false;
    return target.save(blob);
  }

  function defaultExportBaseName(payload, title, defaultValue) {
    const sources = [
      payload && payload.title,
      payload && payload.metadata && payload.metadata.title,
      title
    ];
    for (const source of sources) {
      const value = titleNumberFilenameBase(source);
      if (value) return value;
    }
    return sanitizeFilenameBase(defaultValue || "msa", "msa");
  }

  function titleNumberFilenameBase(title) {
    const text = String(title || "").trim();
    const afterPrefix = text.includes(":") ? text.slice(text.indexOf(":") + 1).trim() : text;
    const match = afterPrefix.match(/(?:^|[^\d])(\d+(?:\.\d+)*)(?=$|[^\d.])/) ||
      text.match(/(?:^|[^\d])(\d+(?:\.\d+)*)(?=$|[^\d.])/);
    return match ? sanitizeFilenameBase(match[1], "") : "";
  }

  function sanitizeFilename(value, defaultValue) {
    const text = String(value || "")
      .trim()
      .replace(/[<>:"/\\|?*\x00-\x1F]+/g, "_")
      .replace(/\s+/g, "_")
      .replace(/^_+|_+$/g, "");
    return text || defaultValue;
  }

  function sanitizeFilenameBase(value, defaultValue) {
    return String(value || "")
      .trim()
      .replace(/[<>:"/\\|?*\x00-\x1F]+/g, "_")
      .replace(/\s+/g, "_")
      .replace(/^_+|_+$/g, "") || defaultValue || "";
  }

  function svgToPNGBlob(svg, width, height, scale) {
    const pixelWidth = Math.ceil(width * scale);
    const pixelHeight = Math.ceil(height * scale);
    const maxPixels = 160000000;
    if (pixelWidth * pixelHeight > maxPixels) {
      throw new Error(`PNG export is too large: ${pixelWidth} x ${pixelHeight} pixels.`);
    }
    return new Promise((resolve, reject) => {
      const image = new Image();
      const url = URL.createObjectURL(new Blob([svg], { type: "image/svg+xml;charset=utf-8" }));
      image.onload = () => {
        try {
          const canvas = document.createElement("canvas");
          canvas.width = pixelWidth;
          canvas.height = pixelHeight;
          const ctx = canvas.getContext("2d");
          if (!ctx) {
            throw new Error("PNG export failed: 2D canvas context is unavailable.");
          }
          ctx.drawImage(image, 0, 0, pixelWidth, pixelHeight);
          canvas.toBlob((blob) => {
            if (blob) {
              resolve(blob);
              return;
            }
            reject(new Error("PNG export failed: the browser did not return a PNG blob."));
          }, "image/png");
        } catch (error) {
          reject(error);
        } finally {
          URL.revokeObjectURL(url);
        }
      };
      image.onerror = () => {
        URL.revokeObjectURL(url);
        reject(new Error("Unable to rasterize SVG for PNG export."));
      };
      image.src = url;
    });
  }

  window.PHGOmsaexpor = {
    DEFAULT_SETTINGS,
    normalizeSettings,
    parseAdvancedLayout,
    buildAutomaticLayout,
    resolveLayout,
    renderScene,
    buildRows,
    defaultExportBaseName,
    titleNumberFilenameBase,
    saveOptionsForSettings,
    prepareSaveTargetForSettings,
    prepareSaveTarget,
    exportScene,
    loadModel,
    renderApp,
    appShell,
    validateModel
  };
})();
