(function() {
  "use strict";

  function formatValue(value) {
    if (value instanceof Error) return value.message;
    if (typeof value === "string") return value;
    if (value && typeof value === "object") {
      const fields = {};
      for (const key of ["message", "name", "stack", "__CLASS_NAME__", "clazzName"]) {
        if (value[key]) fields[key] = String(value[key]);
      }
      try {
        const className = value.getClass$ && value.getClass$().__CLASS_NAME__;
        if (className) fields.className = String(className);
      } catch (_error) {
        // SwingJS exception objects are not always normal JavaScript Errors.
      }
      for (const method of ["getMessage$", "getLocalizedMessage$", "toString$"]) {
        try {
          if (typeof value[method] === "function") {
            const text = value[method]();
            if (text) fields[method] = String(text);
          }
        } catch (_error) {
          // Keep formatting best-effort for bootstrap diagnostics.
        }
      }
      for (const method of ["getName$", "getCanonicalName$", "getSimpleName$"]) {
        try {
          if (typeof value[method] === "function") {
            const text = value[method]();
            if (text) fields[method] = String(text);
          }
        } catch (_error) {
          // Keep formatting best-effort for bootstrap diagnostics.
        }
      }
      if (value.__CLASS_NAME__) fields.rawClassName = String(value.__CLASS_NAME__);
      try {
        const text = value.toString && value.toString();
        if (text && text !== "[object Object]") fields.text = String(text);
      } catch (_error) {
        // Keep formatting best-effort for bootstrap diagnostics.
      }
      if (Object.keys(fields).length > 0) return JSON.stringify(fields);
    }
    try {
      return JSON.stringify(value);
    } catch (_error) {
      return String(value);
    }
  }

  function decodeBase64URL(value) {
    const base64 = String(value || "").replace(/-/g, "+").replace(/_/g, "/");
    const padded = base64 + "=".repeat((4 - (base64.length % 4)) % 4);
    const binary = atob(padded);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i += 1) {
      bytes[i] = binary.charCodeAt(i);
    }
    return new TextDecoder().decode(bytes);
  }

  function parseStateFromHash() {
    if (window.__PHGOJalviewInitialState && typeof window.__PHGOJalviewInitialState === "object") {
      return window.__PHGOJalviewInitialState;
    }
    const hash = String(window.location.hash || "").replace(/^#/, "");
    if (hash.startsWith("phgo=")) {
      return JSON.parse(decodeBase64URL(hash.slice("phgo=".length)));
    }
    const params = new URLSearchParams(hash);
    return {
      open: params.get("open") || "",
      title: params.get("title") || ""
    };
  }

  function notify(type, extra) {
    if (window.parent === window) return;
    const payload = Object.assign({ source: "phgo-jalviewjs", type }, extra || {});
    window.parent.postMessage(payload, "*");
  }

  function debug(type, extra) {
    const payload = Object.assign({ type }, extra || {});
    window.__phgoJalviewDebug = window.__phgoJalviewDebug || [];
    window.__phgoJalviewDebug.push(payload);
    if (window.__phgoJalviewDebug.length > 200) window.__phgoJalviewDebug.shift();
    if (window.console && typeof window.console.debug === "function") {
      window.console.debug("[phgo-jalviewjs]", type, payload);
    }
  }

  function toBootstrapRelativePath(value) {
    const target = String(value || "").trim();
    if (!target) return "";
    if (/^https?:\/\//i.test(target)) return target;
    if (target.startsWith("/")) return `${window.location.origin}${target}`;
    return target;
  }

  function elementState(id) {
    const node = document.getElementById(id);
    if (!node) return null;
    const rect = node.getBoundingClientRect();
    return {
      tag: node.tagName,
      childCount: node.childElementCount,
      htmlLength: node.innerHTML.length,
      width: rect.width,
      height: rect.height
    };
  }

  function collectState() {
    const ids = [
      "jalview-desktop-div",
      "jalview-alignment-div",
      "jalview-structureviewer-div",
      "jalview-tree-div",
      "jalview-pca-div",
      "sysoutdiv"
    ];
    const elements = {};
    for (const id of ids) {
      elements[id] = elementState(id);
    }
    return {
      title: document.title,
      bodyChildCount: document.body ? document.body.childElementCount : 0,
      ids: elements,
      jalviewGlobals: {
        SwingJS: !!window.SwingJS,
        JalviewJSEmbedded: !!window.JalviewJSEmbedded,
        JalviewJS: !!window.JalviewJS
      }
    };
  }

  function viewportSize() {
    return {
      width: Math.max(320, Math.floor(window.innerWidth || document.documentElement.clientWidth || 0)),
      height: Math.max(240, Math.floor(window.innerHeight || document.documentElement.clientHeight || 0))
    };
  }

  function normalizeSequenceKind(value) {
    const kind = String(value || "").trim().toLowerCase();
    if (kind === "protein" || kind === "peptide" || kind === "aa" || kind === "aminoacid" || kind === "amino_acid") {
      return "protein";
    }
    if (kind === "dna" || kind === "rna" || kind === "na" || kind === "nucleotide" || kind === "nucleic") {
      return "nucleotide";
    }
    return "";
  }

  function installPHgoState(state) {
    const conversionTarget = normalizeSequenceKind(state.conversionTarget);
    const sequenceKind = conversionTarget || normalizeSequenceKind(state.sequenceKind);
    window.__PHGOJalviewState = {
      session: String(state.session || ""),
      open: String(state.open || ""),
      title: String(state.title || ""),
      sequenceKind,
      conversionTarget,
      alignmentMethod: String(state.alignmentMethod || ""),
      treeMethod: String(state.treeMethod || ""),
      payloadUpdatedAt: String(state.payloadUpdatedAt || "")
    };
    return window.__PHGOJalviewState;
  }

  function callNoArg(target, names) {
    if (!target) return;
    for (const name of names) {
      if (typeof target[name] === "function") {
        try {
          target[name]();
        } catch (error) {
          debug("layout-call-failed", { method: name, message: formatValue(error) });
        }
      }
    }
  }

  function applyMainFrameMode(frame) {
    const phgo = window.__PHGOJalviewState || {};
    const kind = normalizeSequenceKind(phgo.conversionTarget || phgo.sequenceKind);
    if (!frame || !kind) return;
    try {
      const viewport = typeof frame.getViewport$ === "function" ? frame.getViewport$() : frame.viewport;
      const alignment = viewport && typeof viewport.getAlignment$ === "function" ? viewport.getAlignment$() : viewport && viewport.alignment;
      const nucleotide = kind === "nucleotide";
      if (alignment) {
        alignment.nucleotide = nucleotide;
        if (typeof alignment.getDataset$ === "function") {
          const dataset = alignment.getDataset$();
          if (dataset) dataset.nucleotide = nucleotide;
        }
      }
      frame.__phgoSequenceKind = kind;
      callNoArg(frame, ["setGUINucleotide$", "buildColourMenu$", "setMenusForViewport$", "doLayout$", "validate$", "revalidate$", "repaint$"]);
      if (frame.alignPanel) {
        callNoArg(frame.alignPanel, ["doLayout$", "validate$", "revalidate$", "repaint$"]);
        if (typeof frame.alignPanel.paintAlignment$Z$Z === "function") {
          frame.alignPanel.paintAlignment$Z$Z(true, true);
        }
      }
    } catch (error) {
      debug("apply-mode-failed", { kind, message: formatValue(error) });
    }
  }

  function mainAlignmentFrame(desktop) {
    if (window.__PHGOMainAlignmentFrame) return window.__PHGOMainAlignmentFrame;
    if (desktop && desktop.phgoMainAlignmentFrame) return desktop.phgoMainAlignmentFrame;
    if (desktop && desktop.instance && desktop.instance.phgoMainAlignmentFrame) return desktop.instance.phgoMainAlignmentFrame;
    if (desktop && typeof desktop.getAlignFrames$ === "function") {
      try {
        const frames = desktop.getAlignFrames$() || [];
        for (let i = 0; i < frames.length; i += 1) {
          if (frames[i] && frames[i].__phgoMainAlignmentFrame) return frames[i];
        }
      } catch (error) {
        debug("main-frame-lookup-failed", { message: formatValue(error) });
      }
    }
    return null;
  }

  function sequenceName(seq) {
    if (!seq) return "";
    try {
      if (typeof seq.getName$ === "function") return String(seq.getName$() || "");
    } catch (_error) {
      return "";
    }
    return String(seq.name || seq.id || "");
  }

  function normalizeSelectionState(value) {
    const state = String(value || "").trim().toLowerCase();
    if (state === "yellow" || state === "red") return state;
    return "green";
  }

  function nextSelectionState(value) {
    switch (normalizeSelectionState(value)) {
      case "green":
        return "yellow";
      default:
        return "green";
    }
  }

  function ensureToast() {
    let toast = document.querySelector(".phgo-warning-toast");
    if (toast) return toast;
    toast = document.createElement("section");
    toast.className = "phgo-warning-toast";
    toast.setAttribute("role", "status");
    toast.setAttribute("aria-live", "polite");
    const message = document.createElement("p");
    message.className = "phgo-warning-toast-message";
    toast.appendChild(message);
    document.body.appendChild(toast);
    return toast;
  }

  function showToast(message, persistent) {
    const toast = ensureToast();
    const text = toast.querySelector(".phgo-warning-toast-message");
    text.textContent = String(message || "");
    toast.hidden = !text.textContent;
    if (!persistent && text.textContent) {
      window.clearTimeout(showToast.timer);
      showToast.timer = window.setTimeout(() => {
        toast.hidden = true;
      }, 2400);
    }
  }

  async function fetchJSON(path, options) {
    const response = await fetch(path, Object.assign({ cache: "no-store" }, options || {}));
    if (!response.ok) throw new Error(`${path} failed: ${response.status}`);
    return response.json();
  }

  async function putJSON(path, value, options) {
    const response = await fetch(path, Object.assign({
      method: "PUT",
      cache: "no-store",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(value)
    }, options || {}));
    if (!response.ok) throw new Error(`${path} failed: ${response.status}`);
  }

  async function currentPayloadUpdatedAt(session) {
    const payload = await fetchJSON(`/sessions/${encodeURIComponent(session)}/payload`);
    return String(payload && payload.updated_at || "");
  }

  function currentSession() {
    return String((window.__PHGOJalviewState || {}).session || "").trim();
  }

  async function loadMSASelection() {
    const session = currentSession();
    if (!session) return;
    const data = await fetchJSON(`/sessions/${encodeURIComponent(session)}/msa/selection`);
    const rows = Array.isArray(data.rows) ? data.rows : [];
    const selection = new Map();
    const byName = new Map();
    const byIndex = new Map();
    for (const row of rows) {
      const taxonID = String(row.taxon_id || "").trim();
      const displayName = String(row.display_name || "").trim();
      const displayPrefix = String(row.display_prefix || "").trim();
      if (!taxonID) continue;
      const index = Number.isFinite(row.index) ? row.index : Number.parseInt(row.index, 10);
      const entry = {
        taxonID,
        displayName,
        displayPrefix,
        displayLabel: String(row.display_label || "").trim(),
        canvasItemIndex: Number.parseInt(row.canvas_item_index, 10),
        canvasRow: Number.parseInt(row.canvas_row, 10),
        index: Number.isFinite(index) ? index : -1,
        state: normalizeSelectionState(row.state)
      };
      selection.set(taxonID, entry);
      if (displayName) byName.set(displayName, entry);
      if (entry.index >= 0) byIndex.set(entry.index, entry);
    }
    window.__PHGOMSASelection = selection;
    window.__PHGOMSASelectionByName = byName;
    window.__PHGOMSASelectionByIndex = byIndex;
  }

  function javaListToArray(value, limit) {
    const max = Math.max(0, limit || 250);
    if (!value || max === 0) return [];
    if (Array.isArray(value)) return value.slice(0, max);
    const out = [];
    try {
      if (typeof value.size$ === "function" && typeof value.get$I === "function") {
        const size = Math.min(max, Number(value.size$()) || 0);
        for (let i = 0; i < size; i += 1) out.push(value.get$I(i));
        return out;
      }
    } catch (error) {
      debug("java-list-size-failed", { message: formatValue(error) });
    }
    try {
      if (typeof value.iterator$ === "function") {
        const iterator = value.iterator$();
        while (iterator && typeof iterator.hasNext$ === "function" && iterator.hasNext$() && out.length < max) {
          out.push(iterator.next$());
        }
        return out;
      }
    } catch (error) {
      debug("java-list-iterator-failed", { message: formatValue(error) });
    }
    return out;
  }

  function callValue(target, names) {
    if (!target) return undefined;
    for (const name of names) {
      try {
        if (typeof target[name] === "function") return target[name]();
      } catch (_error) {
        // Jalview/SwingJS getters may throw while frames are still initializing.
      }
    }
    return undefined;
  }

  function primitiveValue(value) {
    if (value == null) return undefined;
    if (typeof value === "string" || typeof value === "boolean") return value;
    if (typeof value === "number") return Number.isFinite(value) ? value : undefined;
    try {
      if (typeof value.booleanValue$ === "function") return !!value.booleanValue$();
      if (typeof value.intValue$ === "function") return Number(value.intValue$());
      if (typeof value.floatValue$ === "function") return Number(value.floatValue$());
      if (typeof value.doubleValue$ === "function") return Number(value.doubleValue$());
    } catch (_error) {
      return undefined;
    }
    return undefined;
  }

  function stringValue(value) {
    if (value == null) return "";
    const primitive = primitiveValue(value);
    if (primitive != null) return String(primitive);
    try {
      if (typeof value.toString$ === "function") return String(value.toString$() || "");
      if (typeof value.toString === "function") return String(value.toString() || "");
    } catch (_error) {
      return "";
    }
    return "";
  }

  function javaMapToObject(value, limit) {
    const max = Math.max(0, limit || 250);
    if (!value || max === 0) return {};
    const out = {};
    try {
      if (typeof value.entrySet$ === "function") {
        const entries = value.entrySet$();
        const iterator = entries && typeof entries.iterator$ === "function" ? entries.iterator$() : null;
        let count = 0;
        while (iterator && typeof iterator.hasNext$ === "function" && iterator.hasNext$() && count < max) {
          const entry = iterator.next$();
          const key = stringValue(callValue(entry, ["getKey$"]));
          const rawValue = callValue(entry, ["getValue$"]);
          if (key) {
            if (rawValue && typeof rawValue.entrySet$ === "function") {
              out[key] = javaMapToObject(rawValue, max);
            } else {
              const primitive = primitiveValue(rawValue);
              out[key] = primitive !== undefined ? primitive : stringValue(rawValue);
            }
          }
          count += 1;
        }
      }
    } catch (error) {
      debug("java-map-collect-failed", { message: formatValue(error) });
    }
    return out;
  }

  function setIfPresent(target, key, value) {
    const primitive = primitiveValue(value);
    if (primitive !== undefined) {
      target[key] = primitive;
      return;
    }
    const text = stringValue(value).trim();
    if (text && text !== "[object Object]") target[key] = text;
  }

  function setHexIfPresent(target, key, value) {
    const colour = javaColorToHex(value, "");
    if (colour) target[key] = colour;
  }

  function stableColourSchemeName(value) {
    if (!value) return "None";
    const direct = stringValue(callValue(value, ["getSchemeName$"])).trim();
    const raw = (direct || stringValue(value)).trim();
    if (!raw || raw === "[object Object]") return "None";
    const lower = raw.toLowerCase();
    const aliases = [
      [/none|no colour|no color/, "None"],
      [/clustalx|clustal/, "Clustalx"],
      [/blosum\s*62|blosum62/, "BLOSUM62 Score"],
      [/percent(age)?\s*identity|pid/, "Percentage Identity"],
      [/zappo/, "Zappo"],
      [/taylor/, "Taylor"],
      [/hydrophobic/, "Hydrophobicity"],
      [/helix/, "Helix Propensity"],
      [/strand/, "Strand Propensity"],
      [/turn/, "Turn Propensity"],
      [/buried/, "Buried Index"],
      [/nucleotide/, "Nucleotide"],
      [/purine|pyrimidine/, "Purine/Pyrimidine"],
      [/rna.*helices|helices/, "RNA Helices"],
      [/t-?coffee|coffee/, "T-Coffee Scores"],
      [/sequence\s*id|idcolour|id colour/, "Sequence ID"],
      [/user defined/, "User Defined"]
    ];
    for (const [pattern, name] of aliases) {
      if (pattern.test(lower)) return name;
    }
    return raw;
  }

  function javaClassName(value) {
    if (!value) return "";
    for (const key of ["__CLASS_NAME__", "clazzName", "className"]) {
      if (value[key]) return String(value[key]);
    }
    try {
      const cls = value.getClass$ && value.getClass$();
      const name = stringValue(callValue(cls, ["getName$", "getCanonicalName$", "getSimpleName$"]));
      if (name) return name;
      if (cls && cls.__CLASS_NAME__) return String(cls.__CLASS_NAME__);
    } catch (_error) {
      // Class metadata is best-effort for SwingJS objects.
    }
    return "";
  }

  function isJalviewInstance(value, className) {
    if (!value || !className) return false;
    try {
      if (window.Clazz && typeof window.Clazz.instanceOf === "function") {
        return !!window.Clazz.instanceOf(value, className);
      }
    } catch (_error) {
      // Fall back to class-name matching.
    }
    return javaClassName(value).includes(className);
  }

  function jalviewSchemeNameForStableName(name) {
    const stable = stableColourSchemeName(name);
    const names = {
      "None": "None",
      "Clustalx": "Clustal",
      "BLOSUM62 Score": "Blosum62",
      "Percentage Identity": "% Identity",
      "Zappo": "Zappo",
      "Taylor": "Taylor",
      "Hydrophobicity": "Hydrophobic",
      "Helix Propensity": "Helix Propensity",
      "Strand Propensity": "Strand Propensity",
      "Turn Propensity": "Turn Propensity",
      "Buried Index": "Buried Index",
      "Nucleotide": "Nucleotide",
      "Purine/Pyrimidine": "Purine/Pyrimidine",
      "RNA Helices": "RNA Helices",
      "T-Coffee Scores": "T-Coffee Scores",
      "Sequence ID": "Sequence ID"
    };
    return names[stable] || name || stable;
  }

  function jalviewSchemeNameFromClass(value) {
    const name = javaClassName(value);
    if (!name) return "";
    const matches = [
      [/ClustalxColourScheme$/, "Clustal"],
      [/Blosum62ColourScheme$/, "Blosum62"],
      [/PIDColourScheme$/, "% Identity"],
      [/ZappoColourScheme$/, "Zappo"],
      [/TaylorColourScheme$/, "Taylor"],
      [/HydrophobicColourScheme$/, "Hydrophobic"],
      [/HelixColourScheme$/, "Helix Propensity"],
      [/StrandColourScheme$/, "Strand Propensity"],
      [/TurnColourScheme$/, "Turn Propensity"],
      [/BuriedColourScheme$/, "Buried Index"],
      [/NucleotideColourScheme$/, "Nucleotide"],
      [/PurinePyrimidineColourScheme$/, "Purine/Pyrimidine"],
      [/RNAHelicesColour$/, "RNA Helices"],
      [/TCoffeeColourScheme$/, "T-Coffee Scores"],
      [/IdColourScheme$/, "Sequence ID"],
      [/UserColourScheme$/, "User Defined"]
    ];
    for (const [pattern, schemeName] of matches) {
      if (pattern.test(name)) return schemeName;
    }
    return "";
  }

  function colourSchemeRawName(scheme) {
    if (!scheme) return "None";
    for (const method of ["getSchemeName$", "getName$"]) {
      const name = stringValue(callValue(scheme, [method])).trim();
      if (name && name !== "[object Object]") return name;
    }
    const byClass = jalviewSchemeNameFromClass(scheme);
    if (byClass) return byClass;
    try {
      const text = stringValue(scheme).trim();
      if (text && text !== "[object Object]") return text;
    } catch (_error) {
      // Keep raw colour-scheme names best-effort.
    }
    return "None";
  }

  function currentColourSchemeState(frame, viewport) {
    const scheme = viewport && callValue(viewport, ["getGlobalColourScheme$"]);
    if (!scheme) return { type: "none", name: "None", display_name: "None" };
    const rawName = colourSchemeRawName(scheme);
    const stableName = stableColourSchemeName(rawName || jalviewSchemeNameFromClass(scheme));
    if (isJalviewInstance(scheme, "jalview.schemes.UserColourScheme")) {
      const appletParameter = stringValue(callValue(scheme, ["toAppletParameter$"])).trim();
      return {
        type: "user_defined",
        name: rawName && rawName !== "None" ? rawName : "User Defined",
        display_name: stableName && stableName !== "None" ? stableName : "User Defined",
        applet_parameter: appletParameter
      };
    }
    return {
      type: "builtin",
      name: jalviewSchemeNameForStableName(rawName),
      display_name: stableName
    };
  }

  function currentColourSchemeName(frame, viewport) {
    const scheme = currentColourSchemeState(frame, viewport);
    return scheme.display_name || stableColourSchemeName(scheme.name);
  }

  function collectNamedValues(target, entries) {
    const out = {};
    for (const [key, names] of entries) {
      setIfPresent(out, key, callValue(target, names));
    }
    return out;
  }

  function mainViewport(frame) {
    if (!frame) return null;
    try {
      if (typeof frame.getViewport$ === "function") return frame.getViewport$();
    } catch (_error) {
      return null;
    }
    return frame.viewport || frame.av || null;
  }

  function mainAlignment(frame) {
    const viewport = mainViewport(frame);
    if (!viewport) return null;
    try {
      if (typeof viewport.getAlignment$ === "function") return viewport.getAlignment$();
    } catch (_error) {
      return null;
    }
    return viewport.alignment || null;
  }

  function collectSelectionRows() {
    const selection = window.__PHGOMSASelection || new Map();
    return [...selection.values()].map((entry) => ({
      taxon_id: entry.taxonID,
      display_name: entry.displayName || "",
      display_prefix: entry.displayPrefix || "",
      display_label: entry.displayLabel || "",
      canvas_item_index: Number.isFinite(entry.canvasItemIndex) ? entry.canvasItemIndex : undefined,
      canvas_row: Number.isFinite(entry.canvasRow) ? entry.canvasRow : undefined,
      index: entry.index,
      state: normalizeSelectionState(entry.state)
    }));
  }

  function alignmentSequences(frame) {
    const alignment = mainAlignment(frame);
    if (!alignment) return [];
    const raw = callValue(alignment, ["getSequences$"]);
    const fromList = javaListToArray(raw, 5000);
    if (fromList.length > 0) return fromList;
    const height = Number(primitiveValue(callValue(alignment, ["getHeight$"]))) || 0;
    const out = [];
    for (let i = 0; i < height; i += 1) {
      try {
        if (typeof alignment.getSequenceAt$I === "function") {
          const seq = alignment.getSequenceAt$I(i);
          if (seq) out.push(seq);
        }
      } catch (_error) {
        // Keep sequence collection best-effort; Jalview may be mid-refresh.
      }
    }
    return out;
  }

  function sequenceTaxonID(seq) {
    if (!seq) return "";
    return String(seq.phgoTaxonID || "").trim();
  }

  function sequenceDescription(seq) {
    return stringValue(callValue(seq, ["getDescription$"]));
  }

  function sequenceText(seq) {
    return stringValue(callValue(seq, ["getSequenceAsString$"]));
  }

  function collectMSASequences(frame) {
    return alignmentSequences(frame).map((seq, index) => {
      const name = sequenceName(seq);
      const entry = selectionEntryForSequence(sequenceTaxonID(seq), name, index);
      return {
        taxon_id: sequenceTaxonID(seq) || (entry && entry.taxonID) || "",
        display_name: name,
        description: sequenceDescription(seq),
        sequence: sequenceText(seq),
        index
      };
    }).filter((row) => row.taxon_id || row.display_name || row.sequence);
  }

  function collectMSASettings(frame) {
    const viewport = mainViewport(frame);
    const settings = {};
    if (!viewport) return settings;
    Object.assign(settings, collectNamedValues(viewport, [
      ["wrap_alignment", ["getWrapAlignment$", "isWrapAlignment$"]],
      ["show_annotations", ["isShowAnnotation$", "getShowAnnotation$"]],
      ["show_boxes", ["getShowBoxes$", "isShowBoxes$"]],
      ["show_text", ["getShowText$", "isShowText$"]],
      ["show_colour_text", ["getShowColourText$", "isShowColourText$"]],
      ["show_sequence_features", ["isShowSequenceFeatures$", "getShowSequenceFeatures$"]],
      ["render_gaps", ["isRenderGaps$", "getRenderGaps$"]],
      ["right_align_ids", ["isRightAlignIds$", "getRightAlignIds$"]],
      ["scale_protein_as_cdna", ["isScaleProteinAsCdna$", "getScaleProteinAsCdna$"]],
      ["char_width", ["getCharWidth$"]],
      ["char_height", ["getCharHeight$"]],
      ["residue_font", ["getFont$"]],
      ["above_pid_threshold", ["getAbovePIDThreshold$"]],
      ["conservation_selected", ["getConservationSelected$"]],
      ["colour_applies_to_all_groups", ["getColourAppliesToAllGroups$"]],
      ["threshold", ["getThreshold$"]]
    ]));
    const scheme = currentColourSchemeState(frame, viewport);
    settings.colour_scheme_name = scheme.display_name || stableColourSchemeName(scheme.name);
    settings.colour_scheme = scheme;
    return settings;
  }

  function collectMSAColours(frame) {
    const viewport = mainViewport(frame);
    if (!viewport) return {};
    const colours = {};
    setHexIfPresent(colours, "text", callValue(viewport, ["getTextColour$"]));
    setHexIfPresent(colours, "conservation_text", callValue(viewport, ["getConservationColour$"]));
    const scheme = currentColourSchemeState(frame, viewport);
    colours.scheme = scheme;
    colours.colour_scheme_name = scheme.display_name || stableColourSchemeName(scheme.name);
    return colours;
  }

  function collectMSAGroups(frame) {
    const alignment = mainAlignment(frame);
    const rawGroups = alignment && callValue(alignment, ["getGroups$"]);
    return javaListToArray(rawGroups, 500).map((group, index) => {
      const out = { index };
      setIfPresent(out, "name", callValue(group, ["getName$"]));
      setIfPresent(out, "start", callValue(group, ["getStartRes$"]));
      setIfPresent(out, "end", callValue(group, ["getEndRes$"]));
      setIfPresent(out, "display_boxes", callValue(group, ["getDisplayBoxes$"]));
      setIfPresent(out, "display_text", callValue(group, ["getDisplayText$"]));
      setIfPresent(out, "colour_text", callValue(group, ["getColourText$"]));
      setIfPresent(out, "description", callValue(group, ["getDescription$"]));
      setIfPresent(out, "colour_scheme", callValue(group, ["getColourScheme$"]));
      setHexIfPresent(out, "outline_colour", callValue(group, ["getOutlineColour$"]));
      setHexIfPresent(out, "text_colour", callValue(group, ["getTextColour$"]));
      setHexIfPresent(out, "id_colour", callValue(group, ["getIdColour$"]));
      const sequences = javaListToArray(callValue(group, ["getSequences$"]), 500)
        .map(sequenceName)
        .filter(Boolean);
      if (sequences.length > 0) out.sequences = sequences;
      return out;
    });
  }

  function collectMSAAnnotations(frame) {
    const alignment = mainAlignment(frame);
    const annotations = alignment && callValue(alignment, ["getAlignmentAnnotation$"]);
    return javaListToArray(annotations, 500).map((annotation, index) => {
      const out = { index };
      setIfPresent(out, "label", callValue(annotation, ["getLabel$"]));
      setIfPresent(out, "description", callValue(annotation, ["getDescription$"]));
      setIfPresent(out, "visible", callValue(annotation, ["isVisible$", "getVisible$"]));
      setIfPresent(out, "below_alignment", callValue(annotation, ["isBelowAlignment$", "getBelowAlignment$"]));
      setIfPresent(out, "graph", callValue(annotation, ["getGraph$"]));
      const sequenceRef = callValue(annotation, ["getSequenceRef$"]);
      const name = sequenceName(sequenceRef);
      if (name) out.sequence = name;
      const cells = javaListToArray(annotation && annotation.annotations, 20000).map((cell, column) => {
        if (!cell) return null;
        const item = { column };
        setIfPresent(item, "display", cell.displayCharacter);
        setIfPresent(item, "description", cell.description);
        setIfPresent(item, "secondary", cell.secondaryStructure);
        setIfPresent(item, "value", cell.value);
        const colour = javaColorToHex(cell.colour, "");
        if (colour) item.colour = colour;
        return item;
      }).filter(Boolean);
      if (cells.length > 0) out.cells = cells;
      setIfPresent(out, "graph_min", annotation.graphMin);
      setIfPresent(out, "graph_max", annotation.graphMax);
      setIfPresent(out, "graph_height", annotation.graphHeight);
      return out;
    });
  }

  function featureListForSequence(seq) {
    const direct = callValue(seq, ["getSequenceFeatures$"]);
    const fromDirect = javaListToArray(direct, 20000);
    if (fromDirect.length > 0) return fromDirect;
    const featureStore = callValue(seq, ["getFeatures$"]);
    let allFeatures = null;
    try {
      if (featureStore && typeof featureStore.getAllFeatures$SA === "function") {
        allFeatures = featureStore.getAllFeatures$SA([]);
      }
    } catch (_error) {
      allFeatures = null;
    }
    return javaListToArray(allFeatures, 20000);
  }

  function collectFeatureDetails(feature) {
    const out = {};
    setIfPresent(out, "type", callValue(feature, ["getType$"]));
    setIfPresent(out, "description", callValue(feature, ["getDescription$"]));
    setIfPresent(out, "begin", callValue(feature, ["getBegin$"]));
    setIfPresent(out, "end", callValue(feature, ["getEnd$"]));
    setIfPresent(out, "score", callValue(feature, ["getScore$"]));
    setIfPresent(out, "feature_group", callValue(feature, ["getFeatureGroup$"]));
    setIfPresent(out, "status", callValue(feature, ["getStatus$"]));
    setIfPresent(out, "strand", callValue(feature, ["getStrand$"]));
    setIfPresent(out, "phase", callValue(feature, ["getPhase$"]));
    setIfPresent(out, "attributes", callValue(feature, ["getAttributes$"]));
    setIfPresent(out, "ena_location", callValue(feature, ["getEnaLocation$"]));
    setHexIfPresent(out, "colour", callValue(feature, ["getColour$", "getColor$"]));
    setIfPresent(out, "style", callValue(feature, ["getStyle$"]));
    if (feature && feature.otherDetails) {
      const other = javaMapToObject(feature.otherDetails, 250);
      if (Object.keys(other).length > 0) out.other_details = other;
    }
    const links = javaListToArray(feature && feature.links, 100).map(stringValue).filter(Boolean);
    if (links.length > 0) out.links = links;
    return out;
  }

  function collectMSAMarkers(frame) {
    const viewport = mainViewport(frame);
    const alignment = mainAlignment(frame);
    const raw =
      callValue(viewport, ["getMarkers$", "getBookmarks$"]) ||
      callValue(alignment, ["getMarkers$", "getBookmarks$"]);
    return javaListToArray(raw, 1000).map((marker, index) => {
      const out = { index };
      Object.assign(out, collectNamedValues(marker, [
        ["name", ["getName$", "getLabel$"]],
        ["description", ["getDescription$"]],
        ["sequence", ["getSequence$", "getSequenceName$"]],
        ["column", ["getColumn$", "getPosition$", "getStart$"]],
        ["end", ["getEnd$"]],
        ["type", ["getType$"]]
      ]));
      setHexIfPresent(out, "colour", callValue(marker, ["getColour$", "getColor$"]));
      return out;
    }).filter((marker) => Object.keys(marker).length > 1);
  }

  function collectMSAFeatures(frame) {
    const out = [];
    const sequences = alignmentSequences(frame);
    for (let index = 0; index < sequences.length && out.length < 20000; index += 1) {
      const seq = sequences[index];
      const name = sequenceName(seq);
      const entry = selectionEntryForSequence(sequenceTaxonID(seq), name, index);
      const features = featureListForSequence(seq);
      for (let featureIndex = 0; featureIndex < features.length && out.length < 20000; featureIndex += 1) {
        const feature = collectFeatureDetails(features[featureIndex]);
        feature.sequence_index = index;
        feature.feature_index = featureIndex;
        feature.taxon_id = sequenceTaxonID(seq) || (entry && entry.taxonID) || "";
        feature.display_name = name;
        out.push(feature);
      }
    }
    return out;
  }

  function collectMSAState(trigger, options) {
    const frame = mainAlignmentFrame(null);
    const full = !options || options.full !== false;
    const state = {
      schema_version: 2,
      updated_at: new Date().toISOString(),
      rows: collectSelectionRows(),
      viewer_state: {
        trigger: String(trigger || ""),
        title: document.title,
        ready: document.body.classList.contains("phgo-jalview-ready")
      }
    };
    if (full) {
      state.sequences = collectMSASequences(frame);
      state.settings = collectMSASettings(frame);
      state.colours = collectMSAColours(frame);
      state.groups = collectMSAGroups(frame);
      state.annotations = collectMSAAnnotations(frame);
      state.features = collectMSAFeatures(frame);
      state.markers = collectMSAMarkers(frame);
    }
    return state;
  }

  async function loadSavedMSAState() {
    const session = currentSession();
    if (!session) return null;
    return fetchJSON(`/sessions/${encodeURIComponent(session)}/msa/state`);
  }

  function findSequenceBySavedReference(frame, ref) {
    const sequences = alignmentSequences(frame);
    const taxon = String(ref && (ref.taxon_id || ref.taxonID) || "").trim();
    const name = String(ref && (ref.display_name || ref.displayName || ref.name || ref.sequence) || "").trim();
    const index = Number.parseInt(ref && (ref.sequence_index ?? ref.index), 10);
    if (Number.isFinite(index) && index >= 0 && index < sequences.length) return sequences[index];
    if (taxon) {
      const found = sequences.find((seq) => sequenceTaxonID(seq) === taxon);
      if (found) return found;
    }
    if (name) {
      const found = sequences.find((seq) => sequenceName(seq) === name);
      if (found) return found;
    }
    return null;
  }

  function hexToAwtColor(hex) {
    const text = String(hex || "").trim();
    const match = text.match(/^#?([0-9a-f]{6})$/i);
    if (!match) return null;
    const value = match[1];
    return awtColor(
      Number.parseInt(value.slice(0, 2), 16),
      Number.parseInt(value.slice(2, 4), 16),
      Number.parseInt(value.slice(4, 6), 16)
    );
  }

  function savedAnnotationKey(item) {
    return [
      String(item && item.label || ""),
      String(item && item.description || ""),
      String(item && item.sequence || ""),
      String(item && item.index || "")
    ].join("\u001f");
  }

  function existingAnnotationKeys(alignment) {
    const keys = new Set();
    const existing = javaListToArray(alignment && callValue(alignment, ["getAlignmentAnnotation$"]), 2000);
    for (const ann of existing) {
      if (ann && ann.__phgoRestoredKey) keys.add(ann.__phgoRestoredKey);
      keys.add(savedAnnotationKey({
        label: stringValue(callValue(ann, ["getLabel$"])) || ann && ann.label,
        description: stringValue(callValue(ann, ["getDescription$"])) || ann && ann.description,
        sequence: sequenceName(callValue(ann, ["getSequenceRef$"]))
      }));
    }
    return keys;
  }

  function buildSavedAnnotation(item, alignmentWidth) {
    const annotationClass = clazzClass("jalview.datamodel.Annotation");
    const alignmentAnnotationClass = clazzClass("jalview.datamodel.AlignmentAnnotation");
    if (!annotationClass || !alignmentAnnotationClass) return null;
    const cells = Array.isArray(item && item.cells) ? item.cells : [];
    const width = Math.max(
      1,
      Number(alignmentWidth) || 0,
      ...cells.map((cell) => (Number.parseInt(cell && cell.column, 10) || 0) + 1)
    );
    const annotations = window.Clazz && typeof window.Clazz.array === "function"
      ? window.Clazz.array(annotationClass, [width])
      : new Array(width);
    for (const cell of cells) {
      const column = Number.parseInt(cell && cell.column, 10);
      if (!Number.isFinite(column) || column < 0 || column >= width) continue;
      const display = String(cell.display || "");
      const desc = String(cell.description || "");
      const secondary = String(cell.secondary || " ").charAt(0) || " ";
      const value = Number(cell.value);
      const colour = hexToAwtColor(cell.colour);
      const ctor = colour && annotationClass.c$$S$S$C$F$java_awt_Color ? annotationClass.c$$S$S$C$F$java_awt_Color : annotationClass.c$$S$S$C$F;
      const args = colour && annotationClass.c$$S$S$C$F$java_awt_Color
        ? [display, desc, secondary, Number.isFinite(value) ? value : 0, colour]
        : [display, desc, secondary, Number.isFinite(value) ? value : 0];
      annotations[column] = clazzNew(ctor, args);
    }
    const label = String(item && item.label || "PHgo annotation");
    const description = String(item && item.description || "");
    const graph = Number(item && item.graph);
    let ann = null;
    if (Number.isFinite(graph) && graph !== 0 && alignmentAnnotationClass.c$$S$S$jalview_datamodel_AnnotationA$F$F$I) {
      const min = Number(item.graph_min);
      const max = Number(item.graph_max);
      ann = clazzNew(alignmentAnnotationClass.c$$S$S$jalview_datamodel_AnnotationA$F$F$I, [
        label,
        description,
        annotations,
        Number.isFinite(min) ? min : 0,
        Number.isFinite(max) ? max : 0,
        graph
      ]);
    }
    if (!ann) {
      ann = clazzNew(alignmentAnnotationClass.c$$S$S$jalview_datamodel_AnnotationA, [label, description, annotations]);
    }
    if (!ann) return null;
    ann.__phgoRestoredKey = savedAnnotationKey(item);
    ann.__phgoRestoredBySave = true;
    if (typeof item.visible === "boolean") ann.visible = item.visible;
    if (typeof item.below_alignment === "boolean") ann.belowAlignment = item.below_alignment;
    const graphHeight = Number(item.graph_height);
    if (Number.isFinite(graphHeight)) ann.graphHeight = graphHeight;
    return ann;
  }

  function applySavedMSAAnnotations(frame, state) {
    const alignment = mainAlignment(frame);
    if (!alignment || !Array.isArray(state && state.annotations) || state.annotations.length === 0) return 0;
    const width = Number(primitiveValue(callValue(alignment, ["getWidth$"]))) || 0;
    const existing = existingAnnotationKeys(alignment);
    let applied = 0;
    for (const item of state.annotations) {
      const key = savedAnnotationKey(item);
      if (existing.has(key)) continue;
      const ann = buildSavedAnnotation(item, width);
      if (!ann) continue;
      const seq = findSequenceBySavedReference(frame, item);
      try {
        if (seq && typeof ann.createSequenceMapping$jalview_datamodel_SequenceI$I$Z === "function") {
          ann.createSequenceMapping$jalview_datamodel_SequenceI$I$Z(seq, 1, true);
        }
        if (typeof alignment.addAnnotation$jalview_datamodel_AlignmentAnnotation === "function") {
          alignment.addAnnotation$jalview_datamodel_AlignmentAnnotation(ann);
        }
        if (Number.isInteger(Number(item.index)) && typeof alignment.setAnnotationIndex$jalview_datamodel_AlignmentAnnotation$I === "function") {
          alignment.setAnnotationIndex$jalview_datamodel_AlignmentAnnotation$I(ann, Number(item.index));
        }
        if (seq && typeof seq.addAlignmentAnnotation$jalview_datamodel_AlignmentAnnotation === "function") {
          seq.addAlignmentAnnotation$jalview_datamodel_AlignmentAnnotation(ann);
        }
        existing.add(key);
        applied += 1;
      } catch (error) {
        debug("msa-state-annotation-restore-failed", { message: formatValue(error), label: String(item && item.label || "") });
      }
    }
    return applied;
  }

  function savedFeatureKey(item) {
    return [
      String(item && (item.taxon_id || item.display_name || item.sequence_index) || ""),
      String(item && item.type || ""),
      String(item && item.begin || ""),
      String(item && item.end || ""),
      String(item && item.feature_group || "")
    ].join("\u001f");
  }

  function removePHgoRestoredAnnotations(frame) {
    const alignment = mainAlignment(frame);
    if (!alignment) return;
    const annotations = javaListToArray(callValue(alignment, ["getAlignmentAnnotation$"]), 2000);
    for (const ann of annotations) {
      if (!ann || !ann.__phgoRestoredBySave) continue;
      try {
        if (typeof alignment.deleteAnnotation$jalview_datamodel_AlignmentAnnotation === "function") {
          alignment.deleteAnnotation$jalview_datamodel_AlignmentAnnotation(ann);
        } else if (typeof alignment.removeAnnotation$jalview_datamodel_AlignmentAnnotation === "function") {
          alignment.removeAnnotation$jalview_datamodel_AlignmentAnnotation(ann);
        }
      } catch (error) {
        debug("msa-state-annotation-cleanup-failed", { message: formatValue(error) });
      }
    }
  }

  function removePHgoRestoredSequenceFeatures(frame) {
    for (const seq of alignmentSequences(frame)) {
      seq.__phgoRestoredFeatureKeys = new Set();
      try {
        const existing = featureListForSequence(seq);
        if (!existing.length) continue;
        const retained = existing.filter((feature) => !(feature && feature.__phgoRestoredBySave));
        if (retained.length === existing.length) continue;
        if (typeof seq.setSequenceFeatures$java_util_List === "function") {
          const arrayListClass = clazzClass("java.util.ArrayList");
          const list = arrayListClass ? clazzNew(arrayListClass.c$, []) : null;
          if (list && typeof list.add$TE === "function") {
            for (const feature of retained) list.add$TE(feature);
            seq.setSequenceFeatures$java_util_List(list);
          }
        } else if (typeof seq.setSequenceFeatures$jalview_datamodel_SequenceFeatureA === "function") {
          seq.setSequenceFeatures$jalview_datamodel_SequenceFeatureA(retained);
        }
      } catch (error) {
        debug("msa-state-feature-cleanup-failed", { message: formatValue(error), sequence: sequenceName(seq) });
      }
    }
  }

  function applySavedMSAFeatures(frame, state) {
    if (!Array.isArray(state && state.features) || state.features.length === 0) return 0;
    const featureClass = clazzClass("jalview.datamodel.SequenceFeature");
    const hashMapClass = clazzClass("java.util.HashMap");
    if (!featureClass) return 0;
    let applied = 0;
    for (const item of state.features) {
      const seq = findSequenceBySavedReference(frame, item);
      if (!seq || typeof seq.addSequenceFeature$jalview_datamodel_SequenceFeature !== "function") continue;
      const key = savedFeatureKey(item);
      seq.__phgoRestoredFeatureKeys = seq.__phgoRestoredFeatureKeys || new Set();
      if (seq.__phgoRestoredFeatureKeys.has(key)) continue;
      const begin = Number.parseInt(item.begin, 10);
      const end = Number.parseInt(item.end, 10);
      if (!Number.isFinite(begin) || !Number.isFinite(end)) continue;
      const score = Number(item.score);
      const feature = clazzNew(featureClass.c$$S$S$I$I$F$S || featureClass.c$$S$S$I$I$S, featureClass.c$$S$S$I$I$F$S
        ? [String(item.type || "feature"), String(item.description || ""), begin, end, Number.isFinite(score) ? score : 0, String(item.feature_group || "")]
        : [String(item.type || "feature"), String(item.description || ""), begin, end, String(item.feature_group || "")]);
      if (!feature) continue;
      try {
        feature.__phgoRestoredBySave = true;
        const featureColour = hexToAwtColor(item.colour);
        if (featureColour && typeof feature.setColour$java_awt_Color === "function") {
          feature.setColour$java_awt_Color(featureColour);
        }
        if (hashMapClass && item.other_details && typeof item.other_details === "object") {
          feature.otherDetails = clazzNew(hashMapClass.c$, []);
          for (const [keyName, value] of Object.entries(item.other_details)) {
            if (feature.otherDetails && typeof feature.otherDetails.put$TK$TV === "function") {
              feature.otherDetails.put$TK$TV(String(keyName), String(value));
            }
          }
        }
        if (Array.isArray(item.links) && typeof feature.addLink$S === "function") {
          for (const link of item.links) feature.addLink$S(String(link));
        }
        seq.addSequenceFeature$jalview_datamodel_SequenceFeature(feature);
        seq.__phgoRestoredFeatureKeys.add(key);
        applied += 1;
      } catch (error) {
        debug("msa-state-feature-restore-failed", { message: formatValue(error), type: String(item.type || "") });
      }
    }
    return applied;
  }

  function applySavedMSAGroups(frame, state) {
    const alignment = mainAlignment(frame);
    if (!alignment || !Array.isArray(state && state.groups) || state.groups.length === 0) return 0;
    const groupClass = clazzClass("jalview.datamodel.SequenceGroup");
    if (!groupClass || typeof alignment.addGroup$jalview_datamodel_SequenceGroup !== "function") return 0;
    const existing = javaListToArray(callValue(alignment, ["getGroups$"]), 1000)
      .map((group) => String(group && group.__phgoRestoredKey || callValue(group, ["getName$"]) || ""))
      .filter(Boolean);
    const existingSet = new Set(existing);
    let applied = 0;
    for (const item of state.groups) {
      const key = String(item && (item.name || item.index) || "");
      if (key && existingSet.has(key)) continue;
      const group = clazzNew(groupClass.c$, []);
      if (!group) continue;
      try {
        if (typeof group.setName$S === "function") group.setName$S(String(item.name || "PHgo group"));
        if (typeof group.setDescription$S === "function") group.setDescription$S(String(item.description || ""));
        if (typeof group.setStartRes$I === "function" && Number.isFinite(Number(item.start))) group.setStartRes$I(Number(item.start));
        if (typeof group.setEndRes$I === "function" && Number.isFinite(Number(item.end))) group.setEndRes$I(Number(item.end));
        if (typeof group.setDisplayBoxes$Z === "function" && typeof item.display_boxes === "boolean") group.setDisplayBoxes$Z(item.display_boxes);
        if (typeof group.setDisplayText$Z === "function" && typeof item.display_text === "boolean") group.setDisplayText$Z(item.display_text);
        if (typeof group.setColourText$Z === "function" && typeof item.colour_text === "boolean") group.setColourText$Z(item.colour_text);
        const outlineColour = hexToAwtColor(item.outline_colour);
        if (outlineColour && typeof group.setOutlineColour$java_awt_Color === "function") group.setOutlineColour$java_awt_Color(outlineColour);
        const textColour = hexToAwtColor(item.text_colour);
        if (textColour && typeof group.setTextColour$java_awt_Color === "function") group.setTextColour$java_awt_Color(textColour);
        const names = Array.isArray(item.sequences) ? item.sequences : [];
        for (const name of names) {
          const seq = findSequenceBySavedReference(frame, { display_name: name, name });
          if (seq && typeof group.addSequence$jalview_datamodel_SequenceI$Z === "function") {
            group.addSequence$jalview_datamodel_SequenceI$Z(seq, false);
          }
        }
        group.__phgoRestoredKey = key || String(item.index || applied);
        group.__phgoRestoredBySave = true;
        alignment.addGroup$jalview_datamodel_SequenceGroup(group);
        existingSet.add(group.__phgoRestoredKey);
        applied += 1;
      } catch (error) {
        debug("msa-state-group-restore-failed", { message: formatValue(error), name: String(item.name || "") });
      }
    }
    return applied;
  }

  function removePHgoRestoredGroups(frame) {
    const alignment = mainAlignment(frame);
    if (!alignment) return;
    const groups = javaListToArray(callValue(alignment, ["getGroups$"]), 2000);
    for (const group of groups) {
      if (!group || !group.__phgoRestoredBySave) continue;
      try {
        if (typeof alignment.deleteGroup$jalview_datamodel_SequenceGroup === "function") {
          alignment.deleteGroup$jalview_datamodel_SequenceGroup(group);
        } else if (typeof alignment.removeGroup$jalview_datamodel_SequenceGroup === "function") {
          alignment.removeGroup$jalview_datamodel_SequenceGroup(group);
        }
      } catch (error) {
        debug("msa-state-group-cleanup-failed", { message: formatValue(error) });
      }
    }
  }

  function applySavedMSASettings(frame, state) {
    const viewport = mainViewport(frame);
    const settings = state && state.settings || {};
    if (!viewport || !settings || typeof settings !== "object") return 0;
    let applied = 0;
    const boolSetters = [
      ["show_annotations", "setShowAnnotation$Z"],
      ["show_sequence_features", "setShowSequenceFeatures$Z"],
      ["show_boxes", "setShowBoxes$Z"],
      ["show_text", "setShowText$Z"],
      ["show_colour_text", "setShowColourText$Z"],
      ["render_gaps", "setRenderGaps$Z"],
      ["wrap_alignment", "setWrapAlignment$Z"],
      ["right_align_ids", "setRightAlignIds$Z"],
      ["above_pid_threshold", "setAbovePIDThreshold$Z"],
      ["conservation_selected", "setConservationSelected$Z"],
      ["colour_applies_to_all_groups", "setColourAppliesToAllGroups$Z"]
    ];
    for (const [key, method] of boolSetters) {
      if (typeof settings[key] === "boolean" && typeof viewport[method] === "function") {
        try {
          viewport[method](settings[key]);
          applied += 1;
        } catch (_error) {
          // Continue with other restorable settings.
        }
      }
    }
    const intSetters = [
      ["char_width", "setCharWidth$I"],
      ["char_height", "setCharHeight$I"]
    ];
    for (const [key, method] of intSetters) {
      const value = Number.parseInt(settings[key], 10);
      if (Number.isFinite(value) && value > 0 && typeof viewport[method] === "function") {
        try {
          viewport[method](value);
          applied += 1;
        } catch (_error) {
          // Continue with other restorable settings.
        }
      }
    }
    return applied;
  }

  function normalizeSavedColourScheme(state) {
    const colours = state && state.colours || {};
    const settings = state && state.settings || {};
    const raw = colours.scheme && typeof colours.scheme === "object"
      ? colours.scheme
      : (settings.colour_scheme && typeof settings.colour_scheme === "object" ? settings.colour_scheme : null);
    if (raw) {
      const type = String(raw.type || "").trim().toLowerCase();
      const name = String(raw.name || raw.display_name || raw.colour_scheme_name || "").trim();
      const appletParameter = String(raw.applet_parameter || raw.appletParameter || "").trim();
      if (type === "user_defined" || appletParameter) {
        return {
          type: "user_defined",
          name: name || "User Defined",
          display_name: raw.display_name || name || "User Defined",
          applet_parameter: appletParameter
        };
      }
      if (type === "none" || stableColourSchemeName(name) === "None") return { type: "none", name: "None", display_name: "None" };
      if (type === "builtin" || name) {
        return {
          type: "builtin",
          name: jalviewSchemeNameForStableName(name),
          display_name: stableColourSchemeName(name)
        };
      }
    }
    const legacyName = stableColourSchemeName(colours.colour_scheme_name || settings.colour_scheme_name);
    if (!legacyName || legacyName === "None") return { type: "none", name: "None", display_name: "None" };
    return {
      type: legacyName === "User Defined" ? "user_defined" : "builtin",
      name: jalviewSchemeNameForStableName(legacyName),
      display_name: legacyName,
      applet_parameter: ""
    };
  }

  function registeredColourScheme(name, frame, viewport) {
    const jalviewName = jalviewSchemeNameForStableName(name);
    if (!jalviewName || jalviewName === "None") return { name: "None", scheme: null, known: true };
    try {
      const schemesClass = clazzClass("jalview.schemes.ColourSchemes");
      const instance = schemesClass && typeof schemesClass.getInstance$ === "function" ? schemesClass.getInstance$() : null;
      if (instance && typeof instance.getColourScheme$S$jalview_api_AlignViewportI$jalview_datamodel_AnnotatedCollectionI$java_util_Map === "function") {
        const alignment = mainAlignment(frame);
        const hidden = viewport && callValue(viewport, ["getHiddenRepSequences$"]);
        const scheme = instance.getColourScheme$S$jalview_api_AlignViewportI$jalview_datamodel_AnnotatedCollectionI$java_util_Map(jalviewName, viewport, alignment, hidden);
        if (scheme) return { name: jalviewName, scheme, known: true };
      }
    } catch (error) {
      debug("msa-state-colour-scheme-lookup-failed", { name: jalviewName, message: formatValue(error) });
    }
    return { name: jalviewName, scheme: null, known: false };
  }

  function userColourSchemeFromSaved(saved) {
    const parameter = String(saved && saved.applet_parameter || "").trim();
    if (!parameter) return null;
    try {
      const userSchemeClass = clazzClass("jalview.schemes.UserColourScheme");
      const scheme = clazzNew(userSchemeClass && userSchemeClass.c$$S, [parameter]);
      if (scheme && saved.name && typeof scheme.setName$S === "function") {
        scheme.setName$S(saved.name);
      }
      return scheme;
    } catch (error) {
      debug("msa-state-user-colour-scheme-create-failed", { message: formatValue(error) });
      return null;
    }
  }

  function applyColourSchemeObject(frame, viewport, scheme, menuName) {
    if (!frame && !viewport) return 0;
    if (frame && menuName && typeof frame.changeColour_actionPerformed$S === "function") {
      frame.changeColour_actionPerformed$S(menuName);
      return 1;
    }
    if (frame && typeof frame.changeColour$jalview_schemes_ColourSchemeI === "function") {
      frame.changeColour$jalview_schemes_ColourSchemeI(scheme);
      return 1;
    }
    if (viewport && typeof viewport.setGlobalColourScheme$jalview_schemes_ColourSchemeI === "function") {
      viewport.setGlobalColourScheme$jalview_schemes_ColourSchemeI(scheme);
      return 1;
    }
    return 0;
  }

  function applySavedColourScheme(frame, state) {
    const viewport = mainViewport(frame);
    if (!viewport) return 0;
    const saved = normalizeSavedColourScheme(state);
    if (!saved || !saved.type) return 0;
    try {
      let applied = 0;
      if (saved.type === "none") {
        applied = applyColourSchemeObject(frame, viewport, null, "None");
      } else if (saved.type === "user_defined") {
        const scheme = userColourSchemeFromSaved(saved);
        if (!scheme) return 0;
        applied = applyColourSchemeObject(frame, viewport, scheme, "");
      } else {
        const resolved = registeredColourScheme(saved.name || saved.display_name, frame, viewport);
        if (!resolved.known) return 0;
        applied = applyColourSchemeObject(frame, viewport, resolved.scheme, resolved.name);
      }
      if (frame && typeof frame.setMenusForViewport$ === "function") frame.setMenusForViewport$();
      return applied;
    } catch (error) {
      debug("msa-state-colour-scheme-apply-failed", { scheme: saved, message: formatValue(error) });
      return 0;
    }
  }

  function applySavedMSAColours(frame, state) {
    const viewport = mainViewport(frame);
    const colours = state && state.colours || {};
    if (!viewport || !colours || typeof colours !== "object") return 0;
    let applied = 0;
    const applyColour = (key, setterNames) => {
      const color = hexToAwtColor(colours[key]);
      if (!color) return 0;
      for (const setterName of setterNames) {
        if (typeof viewport[setterName] !== "function") continue;
        try {
          viewport[setterName](color);
          return 1;
        } catch (_error) {
          // JalviewJS builds do not expose a consistent setter matrix.
        }
      }
      return 0;
    };
    applied += applyColour("text", ["setTextColour$java_awt_Color", "setTextColor$java_awt_Color"]);
    applied += applyColour("conservation_text", ["setConservationColour$java_awt_Color", "setConservationColor$java_awt_Color"]);
    applied += applySavedColourScheme(frame, state);
    return applied;
  }

  function repaintRestoredMSA(frame) {
    try {
      const alignPanel = frame && frame.alignPanel;
      if (alignPanel && typeof alignPanel.validateAnnotationDimensions$Z === "function") alignPanel.validateAnnotationDimensions$Z(false);
      if (alignPanel && typeof alignPanel.paintAlignment$Z$Z === "function") alignPanel.paintAlignment$Z$Z(true, true);
      else if (alignPanel && typeof alignPanel.repaint$ === "function") alignPanel.repaint$();
      if (frame && typeof frame.buildSortByAnnotationScoresMenu$ === "function") frame.buildSortByAnnotationScoresMenu$();
    } catch (error) {
      debug("msa-state-restore-repaint-failed", { message: formatValue(error) });
    }
  }

  function applySavedMSAStateToJalview(state) {
    const frame = mainAlignmentFrame(null);
    const alignment = mainAlignment(frame);
    if (!frame || !alignment || !state || typeof state !== "object") return false;
    const durableCount =
      (Array.isArray(state.annotations) ? state.annotations.length : 0) +
      (Array.isArray(state.features) ? state.features.length : 0) +
      (Array.isArray(state.groups) ? state.groups.length : 0) +
      (Array.isArray(state.markers) ? state.markers.length : 0);
    const signature = JSON.stringify({
      updated_at: state.updated_at || "",
      colours: state && state.colours && typeof state.colours === "object" ? Object.keys(state.colours).sort().map((key) => [key, state.colours[key]]) : [],
      settings: state && state.settings && typeof state.settings === "object" ? Object.keys(state.settings).sort().map((key) => [key, state.settings[key]]) : [],
      annotations: Array.isArray(state.annotations) ? state.annotations.length : 0,
      features: Array.isArray(state.features) ? state.features.length : 0,
      groups: Array.isArray(state.groups) ? state.groups.length : 0,
      markers: Array.isArray(state.markers) ? state.markers.length : 0
    });
    if (window.__PHGOMSAStateRestoreSignature === signature) return true;
    removePHgoRestoredAnnotations(frame);
    removePHgoRestoredSequenceFeatures(frame);
    removePHgoRestoredGroups(frame);
    const applied = applySavedMSASettings(frame, state) +
      applySavedMSAColours(frame, state) +
      applySavedMSAAnnotations(frame, state) +
      applySavedMSAFeatures(frame, state) +
      applySavedMSAGroups(frame, state);
    if (durableCount > 0 && applied === 0) return false;
    window.__PHGOMSAStateRestoreSignature = signature;
    if (applied > 0) {
      repaintRestoredMSA(frame);
      debug("msa-state-restored", { applied });
    }
    return true;
  }

  function scheduleSavedMSAStateRestore(delay, attempt) {
    window.setTimeout(async () => {
      try {
        const state = await loadSavedMSAState();
        const done = applySavedMSAStateToJalview(state);
        if (!done && (attempt || 0) < 6) scheduleSavedMSAStateRestore(500, (attempt || 0) + 1);
      } catch (error) {
        debug("msa-state-restore-load-failed", { message: formatValue(error) });
      }
    }, delay);
  }

  let msaStateSaveTimer = 0;
  let msaStateSaveChain = Promise.resolve();
  async function saveMSAStateNow(trigger, options, fullState) {
    const session = currentSession();
    if (!session) return;
    const full = fullState !== false;
    const saveTask = msaStateSaveChain.catch(() => undefined).then(async () => {
      try {
        await putJSON(`/sessions/${encodeURIComponent(session)}/msa/state`, collectMSAState(trigger, { full }), options);
      } catch (error) {
        debug("msa-state-save-failed", { trigger, message: formatValue(error) });
      }
    });
    msaStateSaveChain = saveTask;
    await saveTask;
  }

  async function saveMSAStateManual() {
    await saveMSAStateNow("manual-save");
    showToast("MSA state saved.", false);
  }

  function scheduleMSAStateSave(trigger, delay, fullState) {
    if (msaStateSaveTimer) window.clearTimeout(msaStateSaveTimer);
    msaStateSaveTimer = window.setTimeout(() => {
      msaStateSaveTimer = 0;
      saveMSAStateNow(trigger || "debounced", undefined, fullState);
    }, Number.isFinite(delay) ? delay : 900);
  }

  function selectionEntryForSequence(taxonID, name, index) {
    const taxonKey = String(taxonID || "").trim();
    const selection = window.__PHGOMSASelection || new Map();
    if (taxonKey && selection.has(taxonKey)) return selection.get(taxonKey);
    const key = String(name || "").trim();
    const byName = window.__PHGOMSASelectionByName || new Map();
    if (key && byName.has(key)) return byName.get(key);
    if (key && selection.has(key)) return selection.get(key);
    const numericIndex = Number.isFinite(index) ? index : Number.parseInt(index, 10);
    const byIndex = window.__PHGOMSASelectionByIndex || new Map();
    if (Number.isFinite(numericIndex) && byIndex.has(numericIndex)) return byIndex.get(numericIndex);
    return null;
  }

  function selectionStateForSequence(taxonID, name, index) {
    const entry = selectionEntryForSequence(taxonID, name, index);
    return normalizeSelectionState(entry && entry.state);
  }

  function invalidateIdCanvas(idCanvas) {
    if (!idCanvas) return;
    idCanvas.fastPaint = false;
    idCanvas.image = null;
    idCanvas.imgHeight = 0;
  }

  function requestMSARepaint() {
    const frame = mainAlignmentFrame(null);
    if (!frame || !frame.alignPanel) return;
    try {
      const ap = frame.alignPanel;
      const idCanvas = ap.idPanel && ap.idPanel.idCanvas ? ap.idPanel.idCanvas : null;
      invalidateIdCanvas(idCanvas);
      if (ap.av && typeof ap.av.setIdWidth$I === "function") ap.av.setIdWidth$I(-1);
      if (typeof ap.calculateIdWidth$ === "function" && ap.idPanel && ap.idPanel.idCanvas && typeof ap.idPanel.idCanvas.setPreferredSize$java_awt_Dimension === "function") {
        ap.idPanel.idCanvas.setPreferredSize$java_awt_Dimension(ap.calculateIdWidth$());
      }
      if (typeof ap.paintAlignment$Z$Z === "function") {
        ap.paintAlignment$Z$Z(false, false);
      } else if (typeof ap.repaint$ === "function") {
        ap.repaint$();
      } else if (idCanvas && typeof idCanvas.repaint$ === "function") {
        idCanvas.repaint$();
      }
    } catch (error) {
      debug("msa-repaint-failed", { message: formatValue(error) });
    }
  }

  function toggleSelectionForSequence(taxonID, name, index, repaint) {
    const entry = selectionEntryForSequence(taxonID, name, index);
    if (!entry) return "green";
    entry.state = nextSelectionState(entry.state);
    const selection = window.__PHGOMSASelection || new Map();
    selection.set(entry.taxonID, entry);
    if (entry.displayName) {
      const byName = window.__PHGOMSASelectionByName || new Map();
      byName.set(entry.displayName, entry);
      window.__PHGOMSASelectionByName = byName;
    }
    if (entry.index >= 0) {
      const byIndex = window.__PHGOMSASelectionByIndex || new Map();
      byIndex.set(entry.index, entry);
      window.__PHGOMSASelectionByIndex = byIndex;
    }
    window.__PHGOMSASelection = selection;
    if (repaint !== false) requestMSARepaint();
    scheduleMSAStateSave("selection-toggle", 250, false);
    return entry.state;
  }

  async function applyMSASelection() {
    const session = currentSession();
    if (!session) return;
    const selection = window.__PHGOMSASelection || new Map();
    const frame = mainAlignmentFrame(null);
    const sequencesByTaxon = new Map();
    const sequencesByIndex = new Map();
    const sequencesByName = new Map();
    for (const seqRow of collectMSASequences(frame)) {
      if (seqRow.taxon_id) sequencesByTaxon.set(seqRow.taxon_id, seqRow);
      if (Number.isFinite(seqRow.index)) sequencesByIndex.set(seqRow.index, seqRow);
      if (seqRow.display_name) sequencesByName.set(seqRow.display_name, seqRow);
    }
    const rows = [...selection.values()].map((entry) => {
      const seqRow = sequencesByTaxon.get(entry.taxonID) || sequencesByIndex.get(entry.index) || sequencesByName.get(entry.displayName) || {};
      return {
        taxon_id: entry.taxonID,
        name: seqRow.display_name || entry.displayName || "",
        display_name: seqRow.display_name || entry.displayName || "",
        description: seqRow.description || "",
        sequence: seqRow.sequence || "",
        index: entry.index,
        state: normalizeSelectionState(entry.state)
      };
    });
    showToast("Refreshing tree and MSA...", true);
    try {
      await saveMSAStateNow("apply", undefined, true);
      await fetchJSON(`/sessions/${encodeURIComponent(session)}/msa/apply`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ rows })
      });
      window.location.replace(`/sessions/${encodeURIComponent(session)}/msa`);
    } catch (error) {
      showToast(`MSA Apply failed: ${formatValue(error)}`, false);
      throw error;
    }
  }

  function installMSAEvents() {
    const session = currentSession();
    if (session) {
      let reloadScheduled = false;
      try {
        const events = new EventSource(`/events/${encodeURIComponent(session)}`);
        events.addEventListener("update", async () => {
          try {
            const status = await fetchJSON(`/sessions/${encodeURIComponent(session)}/status`);
            if (status && status.refreshing) {
              showToast(status.message || "Refreshing tree and MSA...", true);
            } else if (document.querySelector(".phgo-warning-toast:not([hidden])")) {
              showToast("", true);
            }
          } catch (error) {
            debug("msa-status-failed", { message: formatValue(error) });
          }
          if (reloadScheduled) return;
          try {
            const currentUpdatedAt = await currentPayloadUpdatedAt(session);
            const loadedUpdatedAt = String((window.__PHGOJalviewState || {}).payloadUpdatedAt || "");
            if (currentUpdatedAt && loadedUpdatedAt && currentUpdatedAt !== loadedUpdatedAt) {
              reloadScheduled = true;
              showToast("Reloading MSA...", true);
              window.setTimeout(() => {
                window.location.replace(`/sessions/${encodeURIComponent(session)}/msa`);
              }, 100);
            }
          } catch (error) {
            debug("msa-payload-version-check-failed", { message: formatValue(error) });
          }
        });
      } catch (error) {
        debug("msa-events-failed", { message: formatValue(error) });
      }
    }
  }

  function attachPanelToFrame(frame, panel) {
    if (!frame || !panel) {
      return false;
    }
    if (typeof frame.setContentPane$java_awt_Container === "function") {
      frame.setContentPane$java_awt_Container(panel);
      return true;
    }
    const rootPane = typeof frame.getRootPane$ === "function" ? frame.getRootPane$() : null;
    if (rootPane && typeof rootPane.setContentPane$java_awt_Container === "function") {
      rootPane.setContentPane$java_awt_Container(panel);
      return true;
    }
    const contentPane = typeof frame.getContentPane$ === "function" ? frame.getContentPane$() : null;
    if (contentPane && typeof contentPane.add$java_awt_Component$O === "function") {
      contentPane.add$java_awt_Component$O(panel, "Center");
      return true;
    }
    if (contentPane && typeof contentPane.add$java_awt_Component === "function") {
      contentPane.add$java_awt_Component(panel);
      return true;
    }
    if (typeof frame.add$java_awt_Component$O === "function") {
      frame.add$java_awt_Component$O(panel, "Center");
      return true;
    }
    if (typeof frame.add$java_awt_Component === "function") {
      frame.add$java_awt_Component(panel);
      return true;
    }
    return false;
  }

  function addFrameToJalviewDesktop(desktopClass, frame, title, width, height) {
    if (desktopClass && typeof desktopClass.addInternalFrame$javax_swing_JInternalFrame$S$I$I === "function") {
      desktopClass.addInternalFrame$javax_swing_JInternalFrame$S$I$I(frame, title, width, height);
      return;
    }
    throw new Error("Jalview Desktop.addInternalFrame is required to register the msaexpor child window.");
  }

  function clazzClass(name) {
    if (!window.Clazz || typeof window.Clazz._4Name !== "function") return null;
    return window.Clazz._4Name(name, null, null, true) || window.Clazz._4Name(name);
  }

  function clazzNew(method, args) {
    const ctor = window.Clazz && typeof window.Clazz.new_ === "function"
      ? window.Clazz.new_.bind(window.Clazz)
      : (typeof window.Clazz_new_ === "function" ? window.Clazz_new_ : null);
    if (!ctor || !method) return null;
    return ctor(method, args || []);
  }

  function awtColor(red, green, blue) {
    const colorClass = clazzClass("java.awt.Color");
    if (!colorClass) return null;
    return clazzNew(colorClass.c$$I$I$I, [red, green, blue]);
  }

  function normalizeMSAExportSettings(settings) {
    const raw = settings && typeof settings === "object" ? settings : {};
    const scale = [1, 2, 5, 10].includes(Number(raw.scale)) ? Number(raw.scale) : 2;
    return {
      scale,
      cellWidth: Math.max(1, Math.round(Number(raw.cellWidth) || 9)),
      cellHeight: Math.max(1, Math.round(Number(raw.cellHeight) || 13)),
      showPHgoCoordinates: !!raw.showPHgoCoordinates,
      showLengthRatio: !!raw.showLengthRatio,
      showLengthPercent: !!raw.showLengthPercent,
      showAlignmentColumnNumbers: raw.showAlignmentColumnNumbers !== false,
      columnNumberInterval: Math.max(1, Math.round(Number(raw.columnNumberInterval) || 20)),
      showRightResidueNumbers: !!raw.showRightResidueNumbers || !!raw.useAdvancedLayoutScript,
      showGroups: raw.showGroups !== false,
      showFeatures: raw.showFeatures !== false,
      useAdvancedLayoutScript: !!raw.useAdvancedLayoutScript
    };
  }

  function rowLengthStatsFromText(value) {
    const text = String(value || "");
    let last = -1;
    for (let i = text.length - 1; i >= 0; i -= 1) {
      const ch = text.charAt(i);
      if (ch !== "-" && ch !== "." && !/\s/.test(ch)) {
        last = i;
        break;
      }
    }
    let residues = 0;
    let total = 0;
    for (let i = 0; i < text.length; i += 1) {
      const ch = text.charAt(i);
      if (/\s/.test(ch)) continue;
      total += 1;
      if (ch === "-" || ch === ".") continue;
      if (ch === "*" && i === last) continue;
      residues += 1;
    }
    return { residues, total, percent: total ? residues / total * 100 : 0 };
  }

  function rowLengthStatsFromSequence(seq) {
    return rowLengthStatsFromText(sequenceText(seq));
  }

  function buildMSAExportLabel(taxonID, name, index, sequenceValue, settings) {
    const parts = [];
    const displayName = String(name || "").trim() || `row ${index + 1}`;
    if (settings.showPHgoCoordinates && window.__PHGOJalviewBridgeAPI && window.__PHGOJalviewBridgeAPI.displayPrefixForSequence) {
      const prefix = window.__PHGOJalviewBridgeAPI.displayPrefixForSequence(taxonID, displayName, index);
      if (prefix) parts.push(prefix);
    }
    parts.push(displayName);
    if (settings.showLengthRatio || settings.showLengthPercent) {
      const stats = rowLengthStatsFromText(sequenceValue);
      if (settings.showLengthRatio) parts.push(`${stats.residues}/${stats.total}`);
      if (settings.showLengthPercent) parts.push(`${stats.percent.toFixed(1)}%`);
    }
    return parts.filter(Boolean).join(" ");
  }

  function exportLabelForSequence(taxonID, name, index, sequenceValue, settings) {
    return buildMSAExportLabel(
      String(taxonID || ""),
      name,
      Number.isFinite(Number(index)) ? Number(index) : 0,
      sequenceValue,
      normalizeMSAExportSettings(settings || {})
    );
  }

  function exportRowLabel(seq, index, settings) {
    return buildMSAExportLabel(sequenceTaxonID(seq), sequenceName(seq), index, sequenceText(seq), settings);
  }

  function residueNumberAtExportEnd(seq, startBoundary, endBoundary) {
    if (!seq) return "";
    const text = String(sequenceText(seq) || "");
    const start = Math.max(0, Number(startBoundary) || 0);
    const end = Math.min(text.length, Math.max(start, Number(endBoundary) || 0));
    for (let column = end - 1; column >= start; column -= 1) {
      const ch = text.charAt(column);
      if (ch === "-" || ch === "." || /\s/.test(ch)) continue;
      try {
        if (typeof seq.findPosition$I === "function") return String(seq.findPosition$I(column));
      } catch (_error) {
        break;
      }
      return String(column + 1);
    }
    return "";
  }

  function blockRowsToIndexes(block, alignmentHeight) {
    const rows = Array.isArray(block && block.rows) ? block.rows : [];
    const out = [];
    const seen = new Set();
    for (const row of rows) {
      const index = Number(row && row.index);
      if (!Number.isInteger(index) || index < 0 || index >= alignmentHeight || seen.has(index)) continue;
      seen.add(index);
      out.push(index);
    }
    return out;
  }

  function setComponentSize(component, width, height) {
    if (!component) return;
    try {
      if (typeof component.setSize$I$I === "function") component.setSize$I$I(width, height);
    } catch (_error) {
      // Component size is a drawing hint only; continue with the existing size if SwingJS rejects it.
    }
  }

  function componentSize(component) {
    if (!component) return { width: 0, height: 0 };
    let width = 0;
    let height = 0;
    try {
      if (typeof component.getWidth$ === "function") width = Number(component.getWidth$()) || 0;
      if (typeof component.getHeight$ === "function") height = Number(component.getHeight$()) || 0;
    } catch (_error) {
      return { width: 0, height: 0 };
    }
    return { width, height };
  }

  function escapeXML(value) {
    return String(value == null ? "" : value)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;")
      .replace(/'/g, "&apos;");
  }

  function numberAttr(value) {
    const n = Number(value);
    if (!Number.isFinite(n)) return "0";
    return String(Math.round(n * 1000) / 1000);
  }

  function javaColorToHex(color, fallback) {
    if (!color) return fallback || "";
    try {
      const red = Number(primitiveValue(callValue(color, ["getRed$"])));
      const green = Number(primitiveValue(callValue(color, ["getGreen$"])));
      const blue = Number(primitiveValue(callValue(color, ["getBlue$"])));
      if ([red, green, blue].every((value) => Number.isFinite(value))) {
        return `#${[red, green, blue].map((value) => Math.max(0, Math.min(255, Math.round(value))).toString(16).padStart(2, "0")).join("")}`;
      }
    } catch (_error) {
      // Fall through to CSS/string extraction below.
    }
    const text = String(primitiveValue(color) || "");
    const match = text.match(/#([0-9a-f]{6})/i);
    return match ? `#${match[1].toLowerCase()}` : (fallback || "");
  }

  function colorIsVisibleCellFill(hex) {
    const value = String(hex || "").toLowerCase();
    return !!value && value !== "#ffffff" && value !== "#fff" && value !== "transparent";
  }

  function sequenceCharAt(seq, index) {
    if (!seq || index < 0) return "";
    try {
      if (typeof seq.getCharAt$I === "function" && index < (Number(primitiveValue(callValue(seq, ["getLength$"]))) || 0)) {
        return String(seq.getCharAt$I(index) || "");
      }
    } catch (_error) {
      // Fallback to the serialized sequence text.
    }
    return String(sequenceText(seq) || "").charAt(index);
  }

  function sequenceRenderer(seqCanvas) {
    if (!seqCanvas) return null;
    try {
      if (typeof seqCanvas.getSequenceRenderer$ === "function") return seqCanvas.getSequenceRenderer$();
    } catch (_error) {
      // Fall through.
    }
    return seqCanvas.seqRdr || seqCanvas.sequenceRenderer || null;
  }

  function featureRenderer(seqCanvas, alignPanel, enabled) {
    if (!enabled) return null;
    try {
      if (seqCanvas && typeof seqCanvas.getFeatureRenderer$ === "function") return seqCanvas.getFeatureRenderer$();
    } catch (_error) {
      // Fall through.
    }
    try {
      if (alignPanel && typeof alignPanel.getFeatureRenderer$ === "function") return alignPanel.getFeatureRenderer$();
    } catch (_error) {
      // Fall through.
    }
    return seqCanvas && seqCanvas.fr || null;
  }

  function residueCellColour(seqCanvas, alignPanel, viewport, seq, column, exportSettings) {
    const finder = featureRenderer(seqCanvas, alignPanel, exportSettings.showFeatures);
    const renderer = sequenceRenderer(seqCanvas);
    try {
      if (renderer && typeof renderer.getResidueColour$jalview_datamodel_SequenceI$I$jalview_renderer_seqfeatures_FeatureColourFinder === "function") {
        return javaColorToHex(renderer.getResidueColour$jalview_datamodel_SequenceI$I$jalview_renderer_seqfeatures_FeatureColourFinder(seq, column, finder), "");
      }
      if (renderer && typeof renderer.getResidueColour$ === "function") {
        return javaColorToHex(renderer.getResidueColour$(seq, column, finder), "");
      }
    } catch (_error) {
      // Fall through to viewport residue shading.
    }
    try {
      const shading = viewport && typeof viewport.getResidueShading$ === "function" ? viewport.getResidueShading$() : null;
      const ch = sequenceCharAt(seq, column);
      if (shading && typeof shading.findColour$C$I$jalview_datamodel_SequenceI === "function") {
        return javaColorToHex(shading.findColour$C$I$jalview_datamodel_SequenceI(ch, column, seq), "");
      }
      if (shading && typeof shading.findColour$ === "function") {
        return javaColorToHex(shading.findColour$(ch, column, seq), "");
      }
    } catch (_error) {
      return "";
    }
    return "";
  }

  function booleanFormatValue(value, fallback) {
    const normalized = primitiveValue(value);
    return typeof normalized === "boolean" ? normalized : fallback;
  }

  function formatFlag(owner, methods, fallback) {
    return booleanFormatValue(callValue(owner, methods), fallback);
  }

  function sequenceGroupAt(alignment, seq, column) {
    if (!alignment || !seq) return null;
    let rawGroups = null;
    try {
      if (typeof alignment.findAllGroups$jalview_datamodel_SequenceI === "function") {
        rawGroups = alignment.findAllGroups$jalview_datamodel_SequenceI(seq);
      }
    } catch (_error) {
      return null;
    }
    const groups = javaListToArray(rawGroups, 5000);
    for (const group of groups) {
      const start = Number(primitiveValue(callValue(group, ["getStartRes$"])));
      const end = Number(primitiveValue(callValue(group, ["getEndRes$"])));
      if (Number.isFinite(start) && Number.isFinite(end) && start <= column && column <= end) return group;
    }
    return null;
  }

  function residueShaderColour(shader, seq, column) {
    if (!shader || !seq) return null;
    try {
      const scheme = callValue(shader, ["getColourScheme$"]);
      if (scheme == null) return null;
      const residue = sequenceCharAt(seq, column);
      if (typeof shader.findColour$C$I$jalview_datamodel_SequenceI === "function") {
        return shader.findColour$C$I$jalview_datamodel_SequenceI(residue, column, seq);
      }
      if (typeof shader.findColour$ === "function") return shader.findColour$(residue, column, seq);
    } catch (_error) {
      // A missing colour scheme means the normal text colour remains authoritative.
    }
    return null;
  }

  function darkerColourHex(color, fallback) {
    if (!color) return fallback;
    try {
      if (typeof color.darker$ === "function") return javaColorToHex(color.darker$(), fallback);
    } catch (_error) {
      // Use the original scheme colour when the Java colour helper is unavailable.
    }
    return javaColorToHex(color, fallback);
  }

  // Jalview's SequenceRenderer gives a SequenceGroup precedence over the
  // viewport's Format menu state. Keep this decision independent of the export
  // controls: Boxes controls fills, Text controls glyph visibility, and Colour
  // Text controls the glyph colour (with Jalview's darker-on-boxes contrast).
  function residueFormatStyle(viewport, alignment, seq, column) {
    const group = sequenceGroupAt(alignment, seq, column);
    const hasGroup = !!group;
    const showBoxes = hasGroup
      ? formatFlag(group, ["getDisplayBoxes$", "isDisplayBoxes$"], false)
      : formatFlag(viewport, ["getShowBoxes$", "isShowBoxes$"], false);
    const showText = hasGroup
      ? formatFlag(group, ["getDisplayText$", "isDisplayText$"], true)
      : formatFlag(viewport, ["getShowText$", "isShowText$"], true);
    const colourText = hasGroup
      ? formatFlag(group, ["getColourText$", "isColourText$"], false)
      : formatFlag(viewport, ["getColourText$", "isColourText$"], false);
    const shader = hasGroup
      ? callValue(group, ["getGroupColourScheme$"])
      : callValue(viewport, ["getResidueShading$"]);
    const defaultText = hasGroup
      ? javaColorToHex(callValue(group, ["getTextColour$"]), viewportTextColour(viewport))
      : viewportTextColour(viewport);
    const schemeColour = residueShaderColour(shader, seq, column);
    const textFill = colourText && schemeColour
      ? (showBoxes ? darkerColourHex(schemeColour, defaultText) : javaColorToHex(schemeColour, defaultText))
      : defaultText;
    return { showBoxes, showText, textFill };
  }

  function viewportTextColour(viewport) {
    try {
      if (viewport && typeof viewport.getTextColour$ === "function") return javaColorToHex(viewport.getTextColour$(), "#111111");
    } catch (_error) {
      // Fall through.
    }
    return "#111111";
  }

  function viewportFontSpec(viewport) {
    let size = 12;
    let family = "Consolas, 'Courier New', monospace";
    try {
      const font = viewport && typeof viewport.getFont$ === "function" ? viewport.getFont$() : null;
      const rawSize = Number(primitiveValue(callValue(font, ["getSize$"])));
      const rawFamily = String(primitiveValue(callValue(font, ["getFamily$", "getName$"])) || "").trim();
      if (Number.isFinite(rawSize) && rawSize > 0) size = rawSize;
      if (rawFamily) family = `${rawFamily}, Consolas, 'Courier New', monospace`;
    } catch (_error) {
      // Defaults are fine.
    }
    return { size, family };
  }

  function addSVGText(parts, text, x, y, options) {
    if (text == null || text === "") return;
    const opts = options || {};
    const attrs = [
      `x="${numberAttr(x)}"`,
      `y="${numberAttr(y)}"`,
      opts.anchor ? `text-anchor="${escapeXML(opts.anchor)}"` : "",
      opts.fill ? `fill="${escapeXML(opts.fill)}"` : "",
      opts.className ? `class="${escapeXML(opts.className)}"` : ""
    ].filter(Boolean).join(" ");
    parts.push(`<text ${attrs}>${escapeXML(text)}</text>`);
  }

  function addSVGRect(parts, x, y, width, height, options) {
    if (width <= 0 || height <= 0) return;
    const opts = options || {};
    const attrs = [
      `x="${numberAttr(x)}"`,
      `y="${numberAttr(y)}"`,
      `width="${numberAttr(width)}"`,
      `height="${numberAttr(height)}"`,
      opts.fill ? `fill="${escapeXML(opts.fill)}"` : "fill=\"none\"",
      opts.stroke ? `stroke="${escapeXML(opts.stroke)}"` : "",
      opts.strokeWidth ? `stroke-width="${numberAttr(opts.strokeWidth)}"` : "",
      opts.opacity ? `opacity="${numberAttr(opts.opacity)}"` : ""
    ].filter(Boolean).join(" ");
    parts.push(`<rect ${attrs}/>`);
  }

  function groupContainsSequence(group, seq) {
    if (!group || !seq) return false;
    try {
      const map = typeof group.getSequences$java_util_Map === "function" ? group.getSequences$java_util_Map(null) : null;
      if (map && typeof map.contains$O === "function" && map.contains$O(seq)) return true;
    } catch (_error) {
      // Fall through to sequence list.
    }
    const sequences = javaListToArray(callValue(group, ["getSequences$"]), 5000);
    return sequences.some((candidate) => candidate === seq);
  }

  function addGroupOutlines(parts, alignment, indexes, start, endExclusive, gridX, rowStartY, charWidth, charHeight) {
    const groups = javaListToArray(callValue(alignment, ["getGroups$"]), 5000);
    if (!groups.length) return;
    for (const group of groups) {
      const startRes = Number(primitiveValue(callValue(group, ["getStartRes$"])));
      const endRes = Number(primitiveValue(callValue(group, ["getEndRes$"])));
      if (!Number.isFinite(startRes) || !Number.isFinite(endRes) || endRes < start || startRes >= endExclusive) continue;
      const x1 = Math.max(start, startRes);
      const x2 = Math.min(endExclusive - 1, endRes);
      const stroke = javaColorToHex(callValue(group, ["getOutlineColour$"]), "#404040");
      for (let row = 0; row < indexes.length; row += 1) {
        const seq = alignment.getSequenceAt$I(indexes[row]);
        if (!groupContainsSequence(group, seq)) continue;
        addSVGRect(parts, gridX + (x1 - start) * charWidth, rowStartY + row * charHeight, (x2 - x1 + 1) * charWidth, charHeight, {
          stroke,
          strokeWidth: 1,
          fill: "none"
        });
      }
    }
  }

  function renderMSAExportScene(settings, layout) {
    const frame = mainAlignmentFrame(null);
    const alignPanel = frame && frame.alignPanel;
    const viewport = mainViewport(frame);
    const alignment = mainAlignment(frame);
    if (!alignPanel || !viewport || !alignment) {
      throw new Error("Jalview alignment is not ready for MSA export rendering.");
    }
    const seqPanel = typeof alignPanel.getSeqPanel$ === "function" ? alignPanel.getSeqPanel$() : alignPanel.seqPanel;
    const seqCanvas = seqPanel && seqPanel.seqCanvas;
    const idPanel = typeof alignPanel.getIdPanel$ === "function" ? alignPanel.getIdPanel$() : alignPanel.idPanel;
    const idCanvas = idPanel && (typeof idPanel.getIdCanvas$ === "function" ? idPanel.getIdCanvas$() : idPanel.idCanvas);
    if (!seqCanvas || !idCanvas) {
      throw new Error("Jalview sequence and ID canvases are not ready for MSA export rendering.");
    }
    const exportSettings = normalizeMSAExportSettings(settings);
    const blocks = Array.isArray(layout && layout.blocks) ? layout.blocks : [];
    if (blocks.length === 0) throw new Error("No layout blocks to render.");
    const alignmentHeight = Number(primitiveValue(callValue(alignment, ["getHeight$"]))) || alignmentSequences(frame).length;
    const renderedBlocks = blocks.map((block) => ({ block, indexes: blockRowsToIndexes(block, alignmentHeight) }));
    if (renderedBlocks.every((entry) => entry.indexes.length === 0)) {
      throw new Error("No renderable MSA rows in the export layout.");
    }
    const charWidthBefore = Number(primitiveValue(callValue(viewport, ["getCharWidth$"]))) || exportSettings.cellWidth;
    const charHeightBefore = Number(primitiveValue(callValue(viewport, ["getCharHeight$"]))) || exportSettings.cellHeight;
    try {
      if (typeof viewport.setCharWidth$I === "function") viewport.setCharWidth$I(exportSettings.cellWidth);
      if (typeof viewport.setCharHeight$I === "function") viewport.setCharHeight$I(exportSettings.cellHeight);
    } catch (error) {
      debug("msaexpor-char-size-failed", { message: formatValue(error) });
    }
    const charWidth = Number(primitiveValue(callValue(viewport, ["getCharWidth$"]))) || exportSettings.cellWidth;
    const charHeight = Number(primitiveValue(callValue(viewport, ["getCharHeight$"]))) || exportSettings.cellHeight;
    const topNumberHeight = exportSettings.showAlignmentColumnNumbers ? Math.max(18, charHeight + 7) : 4;
    const blockGap = 12;
    const margin = 14;
    const rightNumberWidth = exportSettings.showRightResidueNumbers ? 52 : 0;
    const imageClass = clazzClass("java.awt.image.BufferedImage");
    let scratch = null;
    let scratchGraphics = null;
    let fontMetrics = null;
    if (imageClass) {
      try {
        scratch = clazzNew(imageClass.c$$I$I$I, [16, 16, 1]);
        scratchGraphics = scratch && scratch.getGraphics$();
        if (scratchGraphics) {
          scratchGraphics.setFont$java_awt_Font(viewport.getFont$());
          fontMetrics = scratchGraphics.getFontMetrics$();
        }
      } catch (_error) {
        fontMetrics = null;
      } finally {
        if (scratchGraphics && typeof scratchGraphics.dispose$ === "function") {
          try { scratchGraphics.dispose$(); } catch (_error) {}
        }
      }
    }
    let maxLabelTextWidth = 72;
    for (const entry of renderedBlocks) {
      for (const index of entry.indexes) {
        const seq = alignment.getSequenceAt$I(index);
        const label = exportRowLabel(seq, index, exportSettings);
        const measured = fontMetrics && typeof fontMetrics.stringWidth$S === "function" ? fontMetrics.stringWidth$S(label) : label.length * 7;
        maxLabelTextWidth = Math.max(maxLabelTextWidth, measured);
      }
    }
    const leftLabelPadding = Math.max(24, Math.ceil(charWidth * 3));
    const leftLabelWidth = Math.ceil(Math.max(96, maxLabelTextWidth + leftLabelPadding));
    const maxBlockColumns = Math.max(1, ...renderedBlocks.map((entry) => Number(entry.block.alignmentWidthForNumbering || entry.block.visibleColumnCount || 0)));
    const gridWidth = maxBlockColumns * charWidth;
    const width = Math.ceil(margin * 2 + leftLabelWidth + gridWidth + rightNumberWidth);
    const height = Math.ceil(margin * 2 + renderedBlocks.reduce((sum, entry) => {
      return sum + topNumberHeight + entry.indexes.length * charHeight + blockGap;
    }, 0) - blockGap);
    const bridge = window.__PHGOJalviewBridgeAPI || {};
    const previousExportSettings = bridge.__msaexporRenderSettings;
    const previousExportActive = window.__PHGO_MSAEXPOR_RENDER_ACTIVE__;
    const font = viewportFontSpec(viewport);
    const textFill = viewportTextColour(viewport);
    const renderGaps = primitiveValue(callValue(viewport, ["isRenderGaps$"])) !== false;
    const parts = [
      `<svg xmlns="http://www.w3.org/2000/svg" width="${numberAttr(width)}" height="${numberAttr(height)}" viewBox="0 0 ${numberAttr(width)} ${numberAttr(height)}" data-msaexpor="1" data-msaexpor-renderer="jalview-vector">`,
      `<g font-family="${escapeXML(font.family)}" font-size="${numberAttr(font.size)}" dominant-baseline="alphabetic" letter-spacing="0">`
    ];
    try {
      bridge.__msaexporRenderSettings = exportSettings;
      window.__PHGO_MSAEXPOR_RENDER_ACTIVE__ = true;
      let y = margin;
      for (const entry of renderedBlocks) {
        const block = entry.block;
        const indexes = entry.indexes;
        if (indexes.length === 0) continue;
        const start = Math.max(0, Math.floor(Number(block.columnStartBoundary) || 0));
        const visibleCount = Math.max(0, Math.floor(Number(block.visibleColumnCount) || 0));
        const endExclusive = Math.max(start, Math.floor(Number(block.columnEndBoundary) || (start + visibleCount)));
        const gridX = margin + leftLabelWidth;
        if (exportSettings.showAlignmentColumnNumbers) {
          const numberingColumns = Math.max(visibleCount, Math.floor(Number(block.alignmentWidthForNumbering) || visibleCount));
          for (let col = start + 1; col <= start + numberingColumns; col += 1) {
            if (col % exportSettings.columnNumberInterval !== 0) continue;
            const text = String(col);
            const x = gridX + (col - start - 0.5) * charWidth;
            addSVGText(parts, text, x, y + charHeight, { anchor: "middle", fill: textFill, className: "msaexpor-colnum" });
          }
        }
        const rowStartY = y + topNumberHeight;
        for (let rowIndex = 0; rowIndex < indexes.length; rowIndex += 1) {
          const seqIndex = indexes[rowIndex];
          const rowY = rowStartY + rowIndex * charHeight;
          const seq = alignment.getSequenceAt$I(seqIndex);
          const label = exportRowLabel(seq, seqIndex, exportSettings);
          const baseline = rowY + charHeight - Math.max(2, Math.floor(charHeight / 5));
          addSVGText(parts, label, margin, baseline, { fill: textFill, className: "msaexpor-row-label" });
          for (let col = start; col < endExclusive; col += 1) {
            const ch = sequenceCharAt(seq, col);
            if (!ch) continue;
            const cellX = gridX + (col - start) * charWidth;
            const format = residueFormatStyle(viewport, alignment, seq, col);
            const fill = format.showBoxes ? residueCellColour(seqCanvas, alignPanel, viewport, seq, col, exportSettings) : "";
            if (format.showBoxes && colorIsVisibleCellFill(fill)) {
              addSVGRect(parts, cellX, rowY, charWidth, charHeight, { fill });
            }
            if (format.showText && (renderGaps || (ch !== "-" && ch !== "."))) {
              addSVGText(parts, ch, cellX + charWidth / 2, baseline, { anchor: "middle", fill: format.textFill, className: "msaexpor-residue" });
            }
          }
          if (exportSettings.showRightResidueNumbers) {
            const text = residueNumberAtExportEnd(seq, start, endExclusive);
            if (text) {
              const textWidth = fontMetrics && typeof fontMetrics.stringWidth$S === "function" ? fontMetrics.stringWidth$S(text) : text.length * 7;
              const x = gridX + Math.max(visibleCount, Number(block.alignmentWidthForNumbering) || visibleCount) * charWidth + rightNumberWidth - textWidth - 8;
              addSVGText(parts, text, x, baseline, { fill: textFill, className: "msaexpor-right-number" });
            }
          }
        }
        if (exportSettings.showGroups) {
          addGroupOutlines(parts, alignment, indexes, start, endExclusive, gridX, rowStartY, charWidth, charHeight);
        }
        y += topNumberHeight + indexes.length * charHeight + blockGap;
      }
    } finally {
      bridge.__msaexporRenderSettings = previousExportSettings;
      if (typeof previousExportActive !== "undefined") {
        window.__PHGO_MSAEXPOR_RENDER_ACTIVE__ = previousExportActive;
      } else {
        delete window.__PHGO_MSAEXPOR_RENDER_ACTIVE__;
      }
      try {
        if (typeof viewport.setCharWidth$I === "function") viewport.setCharWidth$I(charWidthBefore);
        if (typeof viewport.setCharHeight$I === "function") viewport.setCharHeight$I(charHeightBefore);
      } catch (error) {
        debug("msaexpor-char-restore-failed", { message: formatValue(error) });
      }
    }
    parts.push("</g></svg>");
    return { svg: parts.join(""), width, height, raster: false, source: "jalview-vector", blocks: blocks.length };
  }

  function createSwingChildWindow(title, width, height) {
    const frameClass = clazzClass("javax.swing.JInternalFrame");
    const panelClass = clazzClass("javax.swing.JPanel");
    const desktopClass = clazzClass("jalview.gui.Desktop");
    const newJavaObject = window.Clazz && typeof window.Clazz.new_ === "function"
      ? window.Clazz.new_.bind(window.Clazz)
      : (typeof window.Clazz_new_ === "function" ? window.Clazz_new_ : null);
    if (!frameClass || !panelClass || !desktopClass || !newJavaObject) {
      throw new Error("SwingJS internal-frame classes are not ready.");
    }
    const frame = newJavaObject(frameClass.c$$S$Z$Z$Z$Z, [title, true, true, true, true]);
    const panel = newJavaObject(panelClass.c$, []);
    panel.__phgoMSAExportPanel = true;
    frame.__phgoMSAExportFrame = true;
    const attached = attachPanelToFrame(frame, panel);
    if (!attached) {
      throw new Error("SwingJS child-window content pane is not available for msaexpor.");
    }
    addFrameToJalviewDesktop(desktopClass, frame, title, width, height);
    return { frame, panel };
  }

  function frameDOMNodeByTitle(title) {
    const wanted = String(title || "").trim();
    const candidates = Array.from(document.querySelectorAll(".swingjs-window, [role='dialog'], [id*='InternalFrame'], [id*='FrameUI'], [id*='RootPaneUI'], [id*='LayeredPaneUI']"));
    const visible = candidates.filter((node) => {
      const rect = node.getBoundingClientRect();
      const style = window.getComputedStyle(node);
      return rect.width > 0 && rect.height > 0 && style.display !== "none" && style.visibility !== "hidden";
    });
    const titled = visible.find((node) => !wanted || String(node.textContent || "").includes(wanted));
    if (titled) return titled;
    const textNodes = Array.from(document.querySelectorAll("body *")).filter((node) => {
      const rect = node.getBoundingClientRect();
      return rect.width > 0 && rect.height > 0 && String(node.textContent || "").trim() === wanted;
    });
    for (const node of textNodes) {
      let parent = node.parentElement;
      while (parent && parent !== document.body) {
        if (parent.matches(".swingjs-window, [role='dialog'], [id*='InternalFrame'], [id*='FrameUI'], [id*='RootPaneUI'], [id*='LayeredPaneUI']")) {
          return parent;
        }
        parent = parent.parentElement;
      }
    }
    return null;
  }

  function panelDOMNode(panel, frame, title) {
    const candidates = [
      panel && panel._j2sNode,
      panel && panel._j2sObject,
      panel && panel.domNode,
      panel && panel.html5Applet,
      panel && panel.ui && panel.ui.domNode,
      panel && panel.ui && panel.ui.jqNode && panel.ui.jqNode[0],
      frame && frame._j2sNode,
      frame && frame._j2sObject,
      frame && frame.domNode,
      frame && frame.ui && frame.ui.domNode,
      frame && frame.ui && frame.ui.jqNode && frame.ui.jqNode[0],
      frameDOMNodeByTitle(title)
    ];
    for (const candidate of candidates) {
      if (candidate instanceof HTMLElement) return candidate;
    }
    const nodes = Array.from(document.querySelectorAll("[id*='JPanel'], .swingjs-component, [data-component]"));
    return nodes.reverse().find((node) => node instanceof HTMLElement && node.id && node.offsetParent !== null) || null;
  }

  function configureMSAExportFrameWindow(frameWindow, bridge) {
    if (!frameWindow) return;
    frameWindow.__PHGO_MSAEXPOR_PARENT_WINDOW__ = window;
    frameWindow.__PHGOJalviewBridgeAPI = bridge || window.__PHGOJalviewBridgeAPI;
    if (typeof window.__PHGO_SAVE_BLOB__ === "function") {
      frameWindow.__PHGO_SAVE_BLOB__ = (blob, filename, options) => window.__PHGO_SAVE_BLOB__(blob, filename, options);
    } else {
      delete frameWindow.__PHGO_SAVE_BLOB__;
    }
  }

  function writeMSAExportIframeDocument(doc) {
    doc.open();
    doc.write("<!doctype html><html class=\"phgo-msaexpor-doc\"><head><meta charset=\"utf-8\"><link rel=\"stylesheet\" href=\"/assets/msaexpor/style.css\"><script>window.__PHGOmsaexporLoadError='';window.addEventListener('error',function(event){window.__PHGOmsaexporLoadError=(event&&event.message)||'MSA export iframe script error';});window.addEventListener('unhandledrejection',function(event){window.__PHGOmsaexporLoadError=(event&&event.reason&&event.reason.message)||String(event&&event.reason||'MSA export iframe rejected');});</script><script src=\"/assets/msaexpor/pdf.js\" onerror=\"window.__PHGOmsaexporLoadError='Failed to load /assets/msaexpor/pdf.js';\"></script><script src=\"/assets/msaexpor/index.js\" onerror=\"window.__PHGOmsaexporLoadError='Failed to load /assets/msaexpor/index.js';\"></script></head><body><div class=\"phgo-msaexpor-root\"></div></body></html>");
    doc.close();
  }

  function waitForMSAExportFrameRuntime(iframe) {
    return new Promise((resolve, reject) => {
      const started = Date.now();
      const tick = () => {
        const frameWindow = iframe.contentWindow;
        const doc = iframe.contentDocument || frameWindow && frameWindow.document;
        if (frameWindow && frameWindow.PHGOmsaexpor && typeof frameWindow.PHGOmsaexpor.renderApp === "function" && doc && doc.querySelector(".phgo-msaexpor-root")) {
          resolve({ frameWindow, doc });
          return;
        }
        const loadError = frameWindow && frameWindow.__PHGOmsaexporLoadError;
        if (loadError) {
          reject(new Error(loadError));
          return;
        }
        if (Date.now() - started > 8000) {
          reject(new Error("Timed out loading the MSA export iframe runtime."));
          return;
        }
        window.setTimeout(tick, 25);
      };
      tick();
    });
  }

  async function mountMSAExportHost(hostParent, session, bridge) {
    let iframe = hostParent.querySelector("iframe.phgo-msaexpor-frame");
    if (!iframe) {
      hostParent.innerHTML = "";
      hostParent.classList.add("phgo-msaexpor-window-host");
      iframe = document.createElement("iframe");
      iframe.className = "phgo-msaexpor-frame";
      iframe.title = "PHgo MSA export image settings";
      iframe.setAttribute("src", "about:blank");
      hostParent.appendChild(iframe);
    }
    const doc = iframe.contentDocument || iframe.contentWindow && iframe.contentWindow.document;
    if (!doc) throw new Error("MSA export iframe is not ready.");
    configureMSAExportFrameWindow(iframe.contentWindow, bridge);
    if (!doc.body || !doc.querySelector(".phgo-msaexpor-root") || !iframe.contentWindow.PHGOmsaexpor || typeof iframe.contentWindow.PHGOmsaexpor.renderApp !== "function") {
      writeMSAExportIframeDocument(doc);
      configureMSAExportFrameWindow(iframe.contentWindow, bridge);
    }
    const runtime = await waitForMSAExportFrameRuntime(iframe);
    configureMSAExportFrameWindow(runtime.frameWindow, bridge);
    const host = runtime.doc.querySelector(".phgo-msaexpor-root");
    if (!host) throw new Error("MSA export iframe host is not ready.");
    runtime.frameWindow.PHGOmsaexpor.renderApp(host, { bridge, session, parentWindow: window });
    return { iframe, host };
  }

  function msaExportWindowRuntimeLooksReady(entry) {
    if (!entry || !entry.hostParent || !document.contains(entry.hostParent)) return false;
    const iframe = entry.hostParent.querySelector("iframe.phgo-msaexpor-frame");
    if (!iframe || !iframe.contentWindow) return false;
    const doc = iframe.contentDocument || iframe.contentWindow.document;
    return !!(doc && doc.querySelector(".phgo-msaexpor-root .msaexpor-app") && iframe.contentWindow.PHGOmsaexpor);
  }

  async function ensureMSAExportWindowMounted(reason) {
    const entry = window.__PHGOMSAExportWindow;
    if (!entry || entry.__remounting || !entry.hostParent || !document.contains(entry.hostParent)) return;
    if (msaExportWindowRuntimeLooksReady(entry)) return;
    entry.__remounting = true;
    try {
      const mounted = await mountMSAExportHost(entry.hostParent, currentSession(), window.__PHGOJalviewBridgeAPI);
      entry.host = mounted.host;
      entry.iframe = mounted.iframe;
      debug("msaexpor-remounted", { reason: reason || "unknown" });
    } catch (error) {
      debug("msaexpor-remount-failed", { reason: reason || "unknown", message: formatValue(error) });
    } finally {
      entry.__remounting = false;
    }
  }

  function startMSAExportMountWatch() {
    const entry = window.__PHGOMSAExportWindow;
    if (!entry || entry.__watchStarted) return;
    entry.__watchStarted = true;
    entry.__watchTimer = window.setInterval(() => ensureMSAExportWindowMounted("interval"), 750);
    if (window.MutationObserver) {
      entry.__watchObserver = new MutationObserver(() => {
        if (entry.__watchQueued) return;
        entry.__watchQueued = true;
        window.setTimeout(() => {
          entry.__watchQueued = false;
          ensureMSAExportWindowMounted("mutation");
        }, 60);
      });
      try {
        entry.__watchObserver.observe(document.body, { childList: true, subtree: true });
      } catch (_error) {
        // The interval watcher remains active.
      }
    }
    window.addEventListener("focus", () => ensureMSAExportWindowMounted("focus"));
  }

  async function openMSAExportImageWindow() {
    const session = currentSession();
    if (!session) throw new Error("MSA export requires an active PHgo session.");
    closeAllSwingMenus("msaexpor-open");
    if (window.__PHGOMSAExportWindow && window.__PHGOMSAExportWindow.host && document.contains(window.__PHGOMSAExportWindow.host)) {
      if (window.__PHGOMSAExportWindow.frame && typeof window.__PHGOMSAExportWindow.frame.toFront$ === "function") {
        try { window.__PHGOMSAExportWindow.frame.toFront$(); } catch (_error) {}
      }
      const mounted = await mountMSAExportHost(window.__PHGOMSAExportWindow.hostParent, session, window.__PHGOJalviewBridgeAPI);
      window.__PHGOMSAExportWindow.host = mounted.host;
      window.__PHGOMSAExportWindow.iframe = mounted.iframe;
      startMSAExportMountWatch();
      return window.__PHGOMSAExportWindow;
    }
    let frame;
    let hostParent;
    try {
      const created = createSwingChildWindow("Export image...", 760, 620);
      frame = created.frame;
      hostParent = panelDOMNode(created.panel, frame, "Export image...");
    } catch (error) {
      debug("msaexpor-swing-window-failed", { message: formatValue(error) });
      throw error;
    }
    if (!hostParent) throw new Error("Unable to locate the Jalview/SwingJS child window host required by msaexpor.");
    const mounted = await mountMSAExportHost(hostParent, session, window.__PHGOJalviewBridgeAPI);
    window.__PHGOMSAExportWindow = { frame, host: mounted.host, iframe: mounted.iframe, hostParent };
    startMSAExportMountWatch();
    return window.__PHGOMSAExportWindow;
  }

  async function openMSAExportImageWindowSafe() {
    try {
      return await openMSAExportImageWindow();
    } catch (error) {
      const message = formatValue(error);
      debug("msaexpor-open-failed", { message });
      showToast(`MSA export window failed: ${message}`, false);
      throw error;
    }
  }

  async function installMSASelectionBridge() {
    try {
      await loadMSASelection();
    } catch (error) {
      debug("msa-selection-load-failed", { message: formatValue(error) });
    }
    installMSAEvents();
    window.setTimeout(requestMSARepaint, 200);
    window.setTimeout(requestMSARepaint, 1000);
    scheduleSavedMSAStateRestore(350, 0);
    scheduleSavedMSAStateRestore(1200, 0);
  }

  function resizeMainAlignmentFrame() {
    const size = viewportSize();
    const alignment = document.getElementById("jalview-alignment-div");
    const desktopNode = document.getElementById("jalview-desktop-div");
    if (alignment) {
      alignment.style.width = `${size.width}px`;
      alignment.style.height = `${size.height}px`;
    }
    if (desktopNode) {
      desktopNode.style.width = `${size.width}px`;
      desktopNode.style.height = `${size.height}px`;
    }
    if (typeof window.__PHGOJalviewResizeMainAlignment === "function") {
      window.__PHGOJalviewResizeMainAlignment();
    }
    window.setTimeout(resyncSwingColourSwatches, 0);
  }

  let resizeTimer = 0;
  function scheduleResize() {
    if (typeof window.__PHGOJalviewScheduleResizeMainAlignment === "function") {
      window.__PHGOJalviewScheduleResizeMainAlignment();
      return;
    }
    if (resizeTimer) window.clearTimeout(resizeTimer);
    resizeTimer = window.setTimeout(() => {
      resizeTimer = 0;
      adaptLayout();
    }, 260);
  }

  function adaptLayout() {
    const desktop = document.getElementById("jalview-desktop-div");
    const alignment = document.getElementById("jalview-alignment-div");
    if (!alignment) return false;
    const alignmentReady = alignment.childElementCount > 0 || alignment.innerHTML.length > 1000;

    if (desktop && alignmentReady) {
      desktop.removeAttribute("aria-hidden");
      desktop.classList.add("phgo-window-manager");
    }

    resizeMainAlignmentFrame();

    if (alignmentReady) {
      document.body.classList.add("phgo-jalview-ready");
    }
    return alignmentReady;
  }

  function closeAllSwingMenus(reason) {
    try {
      if (window.jQuery) {
        window.jQuery(".ui-j2smenu").each(function () {
          const menu = window.jQuery(this).data("ui-j2smenu");
          if (menu && typeof menu.collapseAll === "function") {
            menu.collapseAll({ type: reason || "phgo-close", target: document.body }, true, "phgo");
          }
        });
      }
    } catch (error) {
      debug("hide-menus-failed", { reason, method: "j2smenu.collapseAll", message: formatValue(error) });
    }
    try {
      if (window.J2S && window.J2S.Swing && typeof window.J2S.Swing.hideMenus === "function" && window.J2S.thisApplet) {
        window.J2S.Swing.hideMenus(window.J2S.thisApplet);
      }
    } catch (error) {
      debug("hide-menus-failed", { reason, method: "J2S.Swing.hideMenus", message: formatValue(error) });
    }
    try {
      if (window.Clazz && typeof window.Clazz._4Name === "function") {
        const componentUI = window.Clazz._4Name("swingjs.plaf.JSComponentUI");
        if (componentUI && typeof componentUI.hideMenusAndToolTip$ === "function") componentUI.hideMenusAndToolTip$();
        const popupUI = window.Clazz._4Name("swingjs.plaf.JSPopupMenuUI");
        if (popupUI && typeof popupUI.closeAllMenus$ === "function") popupUI.closeAllMenus$();
      }
    } catch (error) {
      debug("hide-menus-failed", { reason, method: "SwingJS plaf close", message: formatValue(error) });
    }
    try {
      document.querySelectorAll(".swingjsPopupMenu, .swingjs-popup, .swingjs-menu, [role='menu'], [role='j2smenu']").forEach((node) => {
        if (!(node instanceof HTMLElement)) return;
        if (node.closest("[id*='_MenuBarUI']")) return;
        node.style.removeProperty("visibility");
        node.style.display = "none";
      });
      document.querySelectorAll(".ui-j2smenu[aria-hidden='true']").forEach((node) => {
        if (!(node instanceof HTMLElement)) return;
        node.style.removeProperty("visibility");
      });
      document.querySelectorAll(".ui-j2smenu-node.ui-state-active, .ui-j2smenu-node.ui-state-focus").forEach((node) => {
        if (!(node instanceof HTMLElement)) return;
        node.classList.remove("ui-state-active", "ui-state-focus");
      });
    } catch (error) {
      debug("hide-menus-failed", { reason, method: "dom-menu-close-secondary", message: formatValue(error) });
    }
  }

  function hideMenusFromOutsideEvent(event) {
    const target = event && event.target;
    if (!target || !(target instanceof Element)) return;
    if (target.closest(".swingjsPopupMenu, .swingjs-popup, .swingjs-menu, [role='menu'], .ui-j2smenu-node")) {
      return;
    }
    if (target.closest("[id*='_MenuBarUI'], [id*='_MenuUI'], [id*='_MenuItemUI'], [id*='_CheckBoxMenuItemUI'], [id*='_RadioButtonMenuItemUI']")) {
      return;
    }
    closeAllSwingMenus(event.type || "outside");
  }

  function hideMenusOnEscape(event) {
    if (event && event.key === "Escape") closeAllSwingMenus("escape");
  }

  const colourSwatchComponents = new Set();

  function swingComponentDOMNode(component) {
    const candidates = [
      component && component._j2sNode,
      component && component._j2sObject,
      component && component.domNode,
      component && component.html5Applet,
      component && component.ui && component.ui.domNode,
      component && component.ui && component.ui.jqNode && component.ui.jqNode[0],
      component && component.peer && component.peer.domNode
    ];
    for (const candidate of candidates) {
      if (candidate instanceof HTMLElement) return candidate;
    }
    return null;
  }

  function isSwingMenuDOMNode(node) {
    if (!(node instanceof Element)) return false;
    return !!node.closest([
      ".ui-j2smenu",
      ".ui-j2smenu-node",
      ".swingjsPopupMenu",
      ".swingjs-popup",
      ".swingjs-menu",
      "[role='menu']",
      "[id*='_MenuUI']",
      "[id*='_MenuItemUI']",
      "[id*='_PopupMenuUI']",
      "[id*='_MenuBarUI']"
    ].join(","));
  }

  function clearSwingColourSwatchStyle(node) {
    if (!(node instanceof HTMLElement) || !node.dataset.phgoColourSwatch) return;
    delete node.dataset.phgoColourSwatch;
    for (const prop of ["background-color", "background-image", "border", "min-width", "min-height", "box-sizing", "opacity"]) {
      node.style.removeProperty(prop);
    }
  }

  function isSwingColourSwatchComponent(component) {
    if (!component) return false;
    const node = swingComponentDOMNode(component);
    if (isSwingMenuDOMNode(node)) return false;
    let tip = "";
    try {
      tip = String(primitiveValue(callValue(component, ["getToolTipText$"])) || "");
    } catch (_error) {
      tip = "";
    }
    if (!/(colour|color)/i.test(tip)) return false;
    if (/colour|color/i.test(tip) && /(min|max|minimum|maximum|gap|hidden|value|set)/i.test(tip)) return true;
    try {
      const preferred = typeof component.getPreferredSize$ === "function" ? component.getPreferredSize$() : null;
      const width = Number(primitiveValue(callValue(preferred, ["getWidth$"])));
      const height = Number(primitiveValue(callValue(preferred, ["getHeight$"])));
      return Number.isFinite(width) && Number.isFinite(height) && width <= 64 && height <= 32 && /colour|color/i.test(tip);
    } catch (_error) {
      return false;
    }
  }

  function syncSwingColourSwatchComponent(component, color) {
    const node = swingComponentDOMNode(component);
    if (isSwingMenuDOMNode(node)) {
      clearSwingColourSwatchStyle(node);
      colourSwatchComponents.delete(component);
      return;
    }
    if (!isSwingColourSwatchComponent(component)) return;
    colourSwatchComponents.add(component);
    if (!node) return;
    const actual = color || callValue(component, ["getBackground$"]);
    const css = javaColorToHex(actual, "");
    if (!css) return;
    node.dataset.phgoColourSwatch = "true";
    node.style.setProperty("background-color", css, "important");
    node.style.setProperty("background-image", "none", "important");
    node.style.setProperty("border", "1px solid #323130", "important");
    node.style.setProperty("min-width", "40px", "important");
    node.style.setProperty("min-height", "20px", "important");
    node.style.setProperty("box-sizing", "border-box", "important");
    node.style.setProperty("opacity", primitiveValue(callValue(component, ["isEnabled$"])) === false ? "0.45" : "1", "important");
  }

  function resyncSwingColourSwatches() {
    for (const component of Array.from(colourSwatchComponents)) {
      try {
        syncSwingColourSwatchComponent(component, callValue(component, ["getBackground$"]));
      } catch (_error) {
        colourSwatchComponents.delete(component);
      }
    }
  }

  function installSwingColourSwatchFix() {
    const componentClass = clazzClass("javax.swing.JComponent");
    const proto = componentClass && componentClass.prototype;
    if (!proto || proto.__phgoColourSwatchFixInstalled) return !!(proto && proto.__phgoColourSwatchFixInstalled);
    proto.__phgoColourSwatchFixInstalled = true;
    const originalSetBackground = proto.setBackground$java_awt_Color;
    if (typeof originalSetBackground === "function") {
      proto.setBackground$java_awt_Color = function(bg) {
        const result = originalSetBackground.apply(this, arguments);
        window.setTimeout(() => syncSwingColourSwatchComponent(this, bg), 0);
        return result;
      };
    }
    const originalSetEnabled = proto.setEnabled$Z;
    if (typeof originalSetEnabled === "function") {
      proto.setEnabled$Z = function(enabled) {
        const result = originalSetEnabled.apply(this, arguments);
        if (colourSwatchComponents.has(this)) {
          window.setTimeout(() => syncSwingColourSwatchComponent(this, callValue(this, ["getBackground$"])), 0);
        }
        return result;
      };
    }
    if (window.MutationObserver && !window.__PHGOColourSwatchObserver) {
      window.__PHGOColourSwatchObserver = new MutationObserver(() => {
        if (window.__PHGOColourSwatchQueued) return;
        window.__PHGOColourSwatchQueued = true;
        window.setTimeout(() => {
          window.__PHGOColourSwatchQueued = false;
          resyncSwingColourSwatches();
        }, 50);
      });
      try {
        window.__PHGOColourSwatchObserver.observe(document.body, { childList: true, subtree: true });
      } catch (_error) {
        // Timed resyncs still cover ordinary dialog creation.
      }
    }
    window.setInterval(resyncSwingColourSwatches, 1000);
    return true;
  }

  function installLayoutBridge() {
    window.addEventListener("resize", scheduleResize);
    window.addEventListener("load", scheduleResize);
    document.addEventListener("pointerdown", hideMenusFromOutsideEvent, true);
    document.addEventListener("mousedown", hideMenusFromOutsideEvent, true);
    document.addEventListener("touchstart", hideMenusFromOutsideEvent, true);
    document.addEventListener("wheel", hideMenusFromOutsideEvent, true);
    document.addEventListener("keydown", hideMenusOnEscape, true);
    window.addEventListener("message", (event) => {
      const data = event.data || {};
      if (data.source === "phgo-jalview-host" && data.type === "resize") {
        scheduleResize();
      }
    });
    if (window.visualViewport) {
      window.visualViewport.addEventListener("resize", scheduleResize);
    }
    window.setTimeout(adaptLayout, 0);
    window.setTimeout(adaptLayout, 250);
    window.setTimeout(adaptLayout, 1000);
    window.setTimeout(installSwingColourSwatchFix, 0);
    window.setTimeout(installSwingColourSwatchFix, 500);
    window.setTimeout(installSwingColourSwatchFix, 1500);
    installMSASelectionBridge();
  }

  function init() {
    let state = {};
    try {
      state = parseStateFromHash();
    } catch (error) {
      notify("error", { message: `Unable to parse PHgo JalviewJS state: ${formatValue(error)}` });
    }
    const phgoState = installPHgoState(state);
    const openTarget = String(state.open || "").trim();
    const argsTarget = toBootstrapRelativePath(openTarget);
    const title = String(state.title || "PHgo JalviewJS").trim() || "PHgo JalviewJS";
    document.title = title;

    const api = {
      state,
      phgoState,
      openTarget,
      argsTarget,
      args: argsTarget ? ["open", argsTarget] : null,
      title,
      notify,
      collectState,
      adaptLayout,
      installLayoutBridge,
      resizeMainAlignmentFrame,
      applyMSASelection,
      selectionStateForSequence,
      toggleSelectionForSequence,
      collectMSAState,
      renderMSAExportScene,
      saveMSAStateNow,
      saveMSAStateManual,
      openMSAExportImageWindow,
      openMSAExportImageWindowSafe,
      scheduleMSAStateSave,
      closeAllSwingMenus,
      invalidateIdCanvas,
      checkboxColumnWidth: () => 16,
      exportLabelForSequence,
      displayPrefixForSequence: (taxonID, name, index) => {
        const entry = selectionEntryForSequence(taxonID, name, index);
        return entry && entry.displayPrefix ? entry.displayPrefix : "";
      },
      formatValue,
      debug
    };
    window.__PHGOJalviewBridgeAPI = api;
    return api;
  }

  // Kept deliberately small: this lets the vendored bridge's format semantics
  // be regression-tested without a running SwingJS desktop.
  window.PHGOJalviewBridge = {
    init,
    testHooks: { residueFormatStyle }
  };
})();
