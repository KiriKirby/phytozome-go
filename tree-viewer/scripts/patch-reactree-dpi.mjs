import { readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const scriptDir = dirname(fileURLToPath(import.meta.url));
const packageRoot = join(scriptDir, '..', 'node_modules', 'reactreejs');
const files = [join(packageRoot, 'dist', 'index.mjs')];

const dprAnchor = 'const AXIS_H = 24;';
const dprPatch = `${dprAnchor}
    const PHGO_DPR = Math.max(1, Math.min(3, window.devicePixelRatio || 1));`;

const labelOriginal = `      if (lblCanvas.width !== LBL_W) lblCanvas.width = LBL_W;
      if (lblCanvas.height !== H) lblCanvas.height = H;
      const lc = lblCanvas.getContext("2d");
      if (lc) {`;

const labelPatched = `      const lblPixelW = Math.max(1, Math.round(LBL_W * PHGO_DPR));
      const lblPixelH = Math.max(1, Math.round(H * PHGO_DPR));
      if (lblCanvas.width !== lblPixelW) lblCanvas.width = lblPixelW;
      if (lblCanvas.height !== lblPixelH) lblCanvas.height = lblPixelH;
      lblCanvas.style.width = \`\${LBL_W}px\`;
      lblCanvas.style.height = \`\${H}px\`;
      const lc = lblCanvas.getContext("2d");
      if (lc) {
        lc.setTransform(PHGO_DPR, 0, 0, PHGO_DPR, 0, 0);`;

const seqOriginal = `    if (seqCanvas.width !== totalW) seqCanvas.width = totalW;
    if (seqCanvas.height !== H) seqCanvas.height = H;
    const sc = seqCanvas.getContext("2d");
    if (!sc) return;`;

const seqPatched = `    const seqPixelW = Math.max(1, Math.round(totalW * PHGO_DPR));
    const seqPixelH = Math.max(1, Math.round(H * PHGO_DPR));
    if (seqCanvas.width !== seqPixelW) seqCanvas.width = seqPixelW;
    if (seqCanvas.height !== seqPixelH) seqCanvas.height = seqPixelH;
    seqCanvas.style.width = \`\${totalW}px\`;
    seqCanvas.style.height = \`\${H}px\`;
    const sc = seqCanvas.getContext("2d");
    if (!sc) return;
    sc.setTransform(PHGO_DPR, 0, 0, PHGO_DPR, 0, 0);`;

const alignmentPerfHelper = `function prepareAlignmentRows(seqMap, isAA) {
  const source = isAA ? AA_COLORS : NUC_COLORS;
  const fallback = "#64748b";
  const prepared = /* @__PURE__ */ new Map();
  for (const [key, seq] of seqMap) {
    const upper = String(seq || "").toUpperCase();
    const chars = Array.from(upper);
    const colors = new Array(chars.length);
    const gaps = new Uint8Array(chars.length);
    for (let i = 0; i < chars.length; i++) {
      const char = chars[i];
      if (char === "-" || char === ".") {
        gaps[i] = 1;
        colors[i] = "";
      } else {
        colors[i] = source[char] ?? fallback;
      }
    }
    prepared.set(key, { seq: upper, chars, colors, gaps });
  }
  return prepared;
}
`;

const alignmentPerfBlock = `  const alignmentPreparedRef = useRef({ source: null, isAA: null, rows: /* @__PURE__ */ new Map() });
  const getPreparedAlignmentRows = useCallback((seqMap, isAA) => {
    const cached = alignmentPreparedRef.current;
    if (cached.source === seqMap && cached.isAA === isAA) return cached.rows;
    const rows = prepareAlignmentRows(seqMap, isAA);
    alignmentPreparedRef.current = { source: seqMap, isAA, rows };
    return rows;
  }, []);
`;

const exportSizeStateBlock = `  const [exportLongEdge, setExportLongEdge] = useState(numberOr(initialSnapshot.exportLongEdge, 4096));
  const exportLongEdgeRef = useRef(4096);
  exportLongEdgeRef.current = exportLongEdge;
`;

const parseOriginal = `function parseNewick(newick) {
  let index = 0;
  function parseSubtree() {
    const node = {};
    if (newick[index] === "(") {
      index++;
      node.children = [parseSubtree()];
      while (newick[index] === ",") {
        index++;
        node.children.push(parseSubtree());
      }
      if (newick[index] === ")") index++;
    }
    let name = "";
    while (index < newick.length && ![":", ",", ")", "("].includes(newick[index])) {
      name += newick[index++];
    }
    node.name = name.trim();
    if (newick[index] === ":") {
      index++;
      let lengthStr = "";
      while (index < newick.length && ![":", ",", ")", "("].includes(newick[index])) {
        lengthStr += newick[index++];
      }
      node.length = parseFloat(lengthStr);
    }
    return node;
  }
  const tree = parseSubtree();
  return tree;
}`;

const parsePatched = `function parseNewick(newick) {
  let index = 0;
  const DELIMS = [":", ",", ")", "(", ";"];
  function skipWhitespace() {
    while (index < newick.length && /\\s/.test(newick[index])) index++;
  }
  function readQuotedName() {
    let name = "";
    index++;
    while (index < newick.length) {
      const ch = newick[index];
      if (ch === "'") {
        if (newick[index + 1] === "'") {
          name += "'";
          index += 2;
          continue;
        }
        index++;
        break;
      }
      name += ch;
      index++;
    }
    return name;
  }
  function readName() {
    skipWhitespace();
    if (newick[index] === "'") return readQuotedName();
    let name = "";
    while (index < newick.length && !DELIMS.includes(newick[index])) {
      name += newick[index++];
    }
    return name.trim();
  }
  function readLength() {
    skipWhitespace();
    let lengthStr = "";
    while (index < newick.length && !DELIMS.includes(newick[index])) {
      lengthStr += newick[index++];
    }
    return parseFloat(lengthStr.trim());
  }
  function parseSubtree() {
    skipWhitespace();
    const node = {};
    if (newick[index] === "(") {
      index++;
      node.children = [parseSubtree()];
      skipWhitespace();
      while (newick[index] === ",") {
        index++;
        node.children.push(parseSubtree());
        skipWhitespace();
      }
      if (newick[index] === ")") index++;
    }
    skipWhitespace();
    node.name = readName();
    skipWhitespace();
    if (newick[index] === ":") {
      index++;
      node.length = readLength();
    }
    skipWhitespace();
    return node;
  }
  const tree = parseSubtree();
  skipWhitespace();
  if (newick[index] === ";") index++;
  return tree;
}`;

const toNewickOriginal = `function toNewick(node) {
  if (!node.children || !node.children.length) {
    const name2 = (node.name || "").replace(/ /g, "_");
    const len2 = node.length != null && !isNaN(node.length) ? \`:\${node.length}\` : "";
    return \`\${name2}\${len2}\`;
  }
  const inner = node.children.map(toNewick).join(",");
  const name = node.name || "";
  const len = node.length != null && !isNaN(node.length) ? \`:\${node.length}\` : "";
  return \`(\${inner})\${name}\${len}\`;
}`;

const toNewickPatched = `function formatNewickName(name) {
  const value = String(name || "");
  if (!value) return "";
  if (/^[^\\s\\(\\)\\[\\]'":;,]+$/.test(value)) return value;
  return \`'\${value.replace(/'/g, "''")}'\`;
}
function toNewick(node) {
  if (!node.children || !node.children.length) {
    const name2 = formatNewickName(node.name || "");
    const len2 = node.length != null && !isNaN(node.length) ? \`:\${node.length}\` : "";
    return \`\${name2}\${len2}\`;
  }
  const inner = node.children.map(toNewick).join(",");
  const name = formatNewickName(node.name || "");
  const len = node.length != null && !isNaN(node.length) ? \`:\${node.length}\` : "";
  return \`(\${inner})\${name}\${len}\`;
}`;

const displayNameAnchor = `function triggerDownload(url, filename) {
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  setTimeout(() => URL.revokeObjectURL(url), 200);
}`;

const displayNamePatched = `function displayTreeName(name) {
  return String(name || "");
}
function triggerDownload(url, filename) {
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  setTimeout(() => URL.revokeObjectURL(url), 200);
}`;

const triggerDownloadPatched = `function triggerDownload(url, filename) {
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  setTimeout(() => URL.revokeObjectURL(url), 200);
}`;

const triggerDownloadBridgePatched = `async function triggerDownload(url, filename, options) {
  const saveURL = typeof window !== "undefined" ? window.__PHGO_SAVE_URL__ : null;
  if (typeof saveURL === "function") {
    try {
      const saved = await saveURL(url, filename, options || {});
      if (saved === false || saved) {
        setTimeout(() => URL.revokeObjectURL(url), 200);
        return;
      }
    } catch (error) {
      const reportError = typeof window !== "undefined" ? window.__PHGO_REPORT_EXPORT_ERROR__ : null;
      if (typeof reportError === "function") {
        reportError(error);
        return;
      }
      throw error;
    }
  }
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  setTimeout(() => URL.revokeObjectURL(url), 200);
}`;

const exportSvgHelper = `function buildExportSVG(svgEl, fallbackWidth = 800, fallbackHeight = 600, exportLongEdge = 4096) {
  const EXPORT_LONG_EDGE = Math.max(256, Math.min(16384, Number(exportLongEdge) || 4096));
  const width = svgEl.width?.baseVal?.value || svgEl.clientWidth || fallbackWidth;
  const height = svgEl.height?.baseVal?.value || svgEl.clientHeight || fallbackHeight;
  const clone = svgEl.cloneNode(true);
  clone.setAttribute("xmlns", "http://www.w3.org/2000/svg");
  const content = clone.querySelector("g");
  const sourceContent = svgEl.querySelector("g");
  let exportWidth = width;
  let exportHeight = height;
  if (content && sourceContent) {
    const originalTransform = content.getAttribute("transform") || "";
    content.removeAttribute("transform");
    content.setAttribute("data-preview-transform", originalTransform);
    let box = null;
    try {
      box = sourceContent.getBBox();
    } catch {
      box = null;
    }
    if (box && Number.isFinite(box.x) && Number.isFinite(box.y) && box.width > 0 && box.height > 0) {
      const safeScale = EXPORT_LONG_EDGE / Math.max(box.width, box.height);
      exportWidth = Math.max(1, Math.ceil(box.width * safeScale));
      exportHeight = Math.max(1, Math.ceil(box.height * safeScale));
      const tx = -box.x * safeScale;
      const ty = -box.y * safeScale;
      content.setAttribute("transform", \`translate(\${tx},\${ty}) scale(\${safeScale})\`);
    }
  }
  clone.setAttribute("width", String(exportWidth));
  clone.setAttribute("height", String(exportHeight));
  clone.setAttribute("viewBox", \`0 0 \${exportWidth} \${exportHeight}\`);
  return { svgString: new XMLSerializer().serializeToString(clone), width: exportWidth, height: exportHeight };
}
`;

const handleDownloadOriginal = `  const handleDownload = useCallback(async (format) => {
    if (!svgRef.current) return;
    const svgEl = svgRef.current;
    const width = svgEl.width?.baseVal?.value || svgEl.clientWidth || 800;
    const height = svgEl.height?.baseVal?.value || svgEl.clientHeight || 600;
    const clone = svgEl.cloneNode(true);
    clone.setAttribute("xmlns", "http://www.w3.org/2000/svg");
    clone.setAttribute("width", String(width));
    clone.setAttribute("height", String(height));
    const isDark = document.documentElement.getAttribute("data-theme") === "dark";
    const bgColor = isDark ? "#1e293b" : "#ffffff";
    const bgRect = document.createElementNS("http://www.w3.org/2000/svg", "rect");
    bgRect.setAttribute("width", String(width));
    bgRect.setAttribute("height", String(height));
    bgRect.setAttribute("fill", bgColor);
    clone.insertBefore(bgRect, clone.firstChild);
    const svgString = new XMLSerializer().serializeToString(clone);
    if (format === "svg") {
      const blob = new Blob([svgString], { type: "image/svg+xml;charset=utf-8" });
      triggerDownload(URL.createObjectURL(blob), "phylotree.svg");
      return;
    }
    const scale = 2;
    const canvas = document.createElement("canvas");
    canvas.width = width * scale;
    canvas.height = height * scale;
    const ctx = canvas.getContext("2d");
    ctx.scale(scale, scale);
    ctx.fillStyle = bgColor;
    ctx.fillRect(0, 0, width, height);
    await new Promise((resolve) => {
      const img = new Image();
      const svgBlob = new Blob([svgString], { type: "image/svg+xml;charset=utf-8" });
      const url = URL.createObjectURL(svgBlob);
      img.onload = () => {
        ctx.drawImage(img, 0, 0);
        URL.revokeObjectURL(url);
        resolve();
      };
      img.onerror = () => {
        URL.revokeObjectURL(url);
        resolve();
      };
      img.src = url;
    });
    if (format === "png") {
      canvas.toBlob((blob) => {
        if (blob) triggerDownload(URL.createObjectURL(blob), "phylotree.png");
      }, "image/png");
      return;
    }
    const { jsPDF } = await import("jspdf");
    const mmW = width * 0.264583;
    const mmH = height * 0.264583;
    const doc = new jsPDF({
      orientation: width >= height ? "l" : "p",
      unit: "mm",
      format: [mmW, mmH]
    });
    doc.addImage(canvas.toDataURL("image/png"), "PNG", 0, 0, mmW, mmH);
    doc.save("phylotree.pdf");
  }, []);`;

const handleDownloadPatchedLegacy = `  const handleDownload = useCallback(async (format) => {
    if (!svgRef.current) return;
    const svgEl = svgRef.current;
    const width = svgEl.width?.baseVal?.value || svgEl.clientWidth || 800;
    const height = svgEl.height?.baseVal?.value || svgEl.clientHeight || 600;
    const clone = svgEl.cloneNode(true);
    clone.setAttribute("xmlns", "http://www.w3.org/2000/svg");
    clone.setAttribute("width", String(width));
    clone.setAttribute("height", String(height));
    const isDark = document.documentElement.getAttribute("data-theme") === "dark";
    const bgColor = isDark ? "#1e293b" : "#ffffff";
    const bgRect = document.createElementNS("http://www.w3.org/2000/svg", "rect");
    bgRect.setAttribute("width", String(width));
    bgRect.setAttribute("height", String(height));
    bgRect.setAttribute("fill", bgColor);
    clone.insertBefore(bgRect, clone.firstChild);
    const svgString = new XMLSerializer().serializeToString(clone);
    const exportTree = typeof window !== "undefined" ? window.__PHGO_EXPORT_TREE__ : null;
    if (typeof exportTree === "function") {
      try {
        await exportTree({ format, svgString, width, height, bgColor, baseName: "phylotree" });
        return;
      } catch (error) {
        const reportError = typeof window !== "undefined" ? window.__PHGO_REPORT_EXPORT_ERROR__ : null;
        if (typeof reportError === "function") {
          reportError(error);
          return;
        }
        throw error;
      }
    }
    if (format === "svg") {
      const blob = new Blob([svgString], { type: "image/svg+xml;charset=utf-8" });
      triggerDownload(URL.createObjectURL(blob), "phylotree.svg");
      return;
    }
    const scale = 10;
    const canvas = document.createElement("canvas");
    canvas.width = width * scale;
    canvas.height = height * scale;
    const ctx = canvas.getContext("2d");
    ctx.scale(scale, scale);
    ctx.fillStyle = bgColor;
    ctx.fillRect(0, 0, width, height);
    await new Promise((resolve) => {
      const img = new Image();
      const svgBlob = new Blob([svgString], { type: "image/svg+xml;charset=utf-8" });
      const url = URL.createObjectURL(svgBlob);
      img.onload = () => {
        ctx.drawImage(img, 0, 0);
        URL.revokeObjectURL(url);
        resolve();
      };
      img.onerror = () => {
        URL.revokeObjectURL(url);
        resolve();
      };
      img.src = url;
    });
    if (format === "png") {
      canvas.toBlob((blob) => {
        if (blob) triggerDownload(URL.createObjectURL(blob), "phylotree.png");
      }, "image/png");
      return;
    }
    const { jsPDF } = await import("jspdf");
    const mmW = width * 0.264583;
    const mmH = height * 0.264583;
    const doc = new jsPDF({
      orientation: width >= height ? "l" : "p",
      unit: "mm",
      format: [mmW, mmH]
    });
    doc.addImage(canvas.toDataURL("image/png"), "PNG", 0, 0, mmW, mmH);
    doc.save("phylotree.pdf");
  }, []);`;

const handleDownloadPatched = `  const handleDownload = useCallback(async (format) => {
    if (!svgRef.current) return;
    const svgEl = svgRef.current;
    const { svgString, width, height } = buildExportSVG(svgEl, 800, 600, exportLongEdgeRef.current);
    const exportTree = typeof window !== "undefined" ? window.__PHGO_EXPORT_TREE__ : null;
    if (typeof exportTree === "function") {
      try {
        await exportTree({ format, svgString, width, height, baseName: "phylotree" });
        return;
      } catch (error) {
        const reportError = typeof window !== "undefined" ? window.__PHGO_REPORT_EXPORT_ERROR__ : null;
        if (typeof reportError === "function") {
          reportError(error);
          return;
        }
        throw error;
      }
    }
    if (format === "svg") {
      const blob = new Blob([svgString], { type: "image/svg+xml;charset=utf-8" });
      triggerDownload(URL.createObjectURL(blob), "phylotree.svg");
      return;
    }
    const scale = 10;
    const canvas = document.createElement("canvas");
    canvas.width = width * scale;
    canvas.height = height * scale;
    const ctx = canvas.getContext("2d");
    ctx.scale(scale, scale);
    ctx.clearRect(0, 0, width, height);
    await new Promise((resolve) => {
      const img = new Image();
      const svgBlob = new Blob([svgString], { type: "image/svg+xml;charset=utf-8" });
      const url = URL.createObjectURL(svgBlob);
      img.onload = () => {
        ctx.drawImage(img, 0, 0);
        URL.revokeObjectURL(url);
        resolve();
      };
      img.onerror = () => {
        URL.revokeObjectURL(url);
        resolve();
      };
      img.src = url;
    });
    if (format === "png") {
      canvas.toBlob((blob) => {
        if (blob) triggerDownload(URL.createObjectURL(blob), "phylotree.png");
      }, "image/png");
      return;
    }
    const { jsPDF } = await import("jspdf");
    const mmW = width * 0.264583;
    const mmH = height * 0.264583;
    const doc = new jsPDF({
      orientation: width >= height ? "l" : "p",
      unit: "mm",
      format: [mmW, mmH]
    });
    doc.addImage(canvas.toDataURL("image/png"), "PNG", 0, 0, mmW, mmH);
    doc.save("phylotree.pdf");
  }, []);`;

const handleDownloadOriginalCJS = handleDownloadOriginal.replace(
  '  const handleDownload = useCallback(async (format) => {',
  '  const handleDownload = (0, import_react.useCallback)(async (format) => {',
);

const handleDownloadPatchedLegacyCJS = handleDownloadPatchedLegacy.replace(
  '  const handleDownload = useCallback(async (format) => {',
  '  const handleDownload = (0, import_react.useCallback)(async (format) => {',
);

const handleDownloadPatchedCJS = handleDownloadPatched.replace(
  '  const handleDownload = useCallback(async (format) => {',
  '  const handleDownload = (0, import_react.useCallback)(async (format) => {',
);

const labelModeGuardOriginalMJS = `  useEffect(() => {
    if (labelMode === "bootstrap" && !hasBootstraps) setLabelMode("none");
    if (labelMode === "branchlength" && !hasBranchLengths) setLabelMode("none");
  }, [hasBootstraps, hasBranchLengths, labelMode]);`;

const labelModeGuardPatchedMJS = `  useLayoutEffect(() => {
    const nextLabelMode = sanitizeLabelMode(labelMode, hasBootstraps, hasBranchLengths);
    if (nextLabelMode !== labelMode) setLabelMode(nextLabelMode);
  }, [hasBootstraps, hasBranchLengths, labelMode]);`;

const labelModeGuardOriginalCJS = `  (0, import_react.useEffect)(() => {
    if (labelMode === "bootstrap" && !hasBootstraps) setLabelMode("none");
    if (labelMode === "branchlength" && !hasBranchLengths) setLabelMode("none");
  }, [hasBootstraps, hasBranchLengths, labelMode]);`;

const labelModeGuardPatchedCJS = `  (0, import_react.useLayoutEffect)(() => {
    const nextLabelMode = sanitizeLabelMode(labelMode, hasBootstraps, hasBranchLengths);
    if (nextLabelMode !== labelMode) setLabelMode(nextLabelMode);
  }, [hasBootstraps, hasBranchLengths, labelMode]);`;

const truncateStateOriginalMJS = '  const [truncateNames, setTruncateNames] = useState(true);';
const truncateStateReplacementMJS = '  const [truncateNames, setTruncateNames] = useState(false);';

const truncateStateOriginalCJS = '  const [truncateNames, setTruncateNames] = (0, import_react.useState)(true);';
const truncateStateReplacementCJS = '  const [truncateNames, setTruncateNames] = (0, import_react.useState)(false);';
const labelModeInitialPatched = '  const [labelMode, setLabelMode] = useState(stringOr(initialSnapshot.labelMode, "bootstrap"));';
const labelModeInitialRootFixed = '  const [labelMode, setLabelMode] = useState(() => sanitizeLabelMode(stringOr(initialSnapshot.labelMode, "bootstrap"), treeHasBootstraps(treeData), treeHasBranchLengths(treeData)));';
const restoreTreeDataPatched = '    dispatch({ type: "RESET", data: snapshot.treeData ? cloneTreeData(snapshot.treeData) : parseNewick(newick) });';
const restoreTreeDataRootFixed = `    const restoredTreeData = snapshot.treeData ? cloneTreeData(snapshot.treeData) : parseNewick(newick);
    const restoredHasBootstraps = treeHasBootstraps(restoredTreeData);
    const restoredHasBranchLengths = treeHasBranchLengths(restoredTreeData);
    dispatch({ type: "RESET", data: restoredTreeData });`;
const restoreLabelModePatched = '    setLabelMode(stringOr(snapshot.labelMode, "bootstrap"));';
const restoreLabelModeRootFixed = '    setLabelMode(sanitizeLabelMode(stringOr(snapshot.labelMode, "bootstrap"), restoredHasBootstraps, restoredHasBranchLengths));';
const restoreEffectDepsPatched = '  }, [newick, initialState, defaultHeight]);';
const restoreEffectDepsRootFixed = '  }, [newick, initialState]);';
const viewportScaleStateAnchor = `  const [strokeWidth, setStrokeWidth] = useState(numberOr(initialSnapshot.strokeWidth, 1.5));
  const vScaleRef = useRef(1);`;
const viewportScaleStatePatched = `  const [strokeWidth, setStrokeWidth] = useState(numberOr(initialSnapshot.strokeWidth, 1.5));
  const [viewportScale, setViewportScale] = useState(numberOr(initialSnapshot.transform?.k, 1));
  const vScaleRef = useRef(1);`;
const viewportScaleRestoreAnchor = '    setStrokeWidth(numberOr(snapshot.strokeWidth, 1.5));';
const viewportScaleRestorePatched = `    setStrokeWidth(numberOr(snapshot.strokeWidth, 1.5));
    setViewportScale(numberOr(snapshot.transform?.k, 1));`;
const zoomEventAnchor = `      transformRef.current = ev.transform;
      content.attr("transform", ev.transform.toString());
      drawAlnCanvasRef.current();`;
const zoomEventPatched = `      transformRef.current = ev.transform;
      setViewportScale(ev.transform.k);
      content.attr("transform", ev.transform.toString());
      drawAlnCanvasRef.current();`;
const zoomEventStatePersistPrevious = `      transformRef.current = ev.transform;
      setViewportScale(ev.transform.k);
      scheduleStateChange();
      content.attr("transform", ev.transform.toString());
      drawAlnCanvasRef.current();`;
const zoomEventStatePersistPatched = `      transformRef.current = ev.transform;
      scheduleViewportScaleChange(ev.transform.k);
      scheduleStateChange();
      content.attr("transform", ev.transform.toString());
      scheduleAlnCanvasDraw();`;
const zoomFilterPatched = '    const zoom2 = d32.zoom().scaleExtent([0.25, 4]).filter((ev) => ev.type !== "dblclick").on("zoom", (ev) => {';
const zoomFilterRootFixed = '    const zoom2 = d32.zoom().scaleExtent([0.25, 4]).filter((ev) => ev.type !== "dblclick" && ev.type !== "wheel").on("zoom", (ev) => {';
const zoomWheelAnchor = `    svg.call(zoom2);
    zoomRef.current = zoom2;
    svg.on("click.nodeInfo", () => setNodeInfoRef.current(null));`;
const zoomWheelPrevious = `    svg.call(zoom2);
    zoomRef.current = zoom2;
    svg.on("wheel.phgoViewport", (event) => {
      event.preventDefault();
      const currentTransform = transformRef.current ?? d32.zoomIdentity;
      if (event.ctrlKey || event.metaKey) {
        const rect = svgRef.current?.getBoundingClientRect();
        if (!rect) return;
        const px = event.clientX - rect.left;
        const py = event.clientY - rect.top;
        const nextScale = Math.max(0.25, Math.min(4, currentTransform.k * Math.exp(-event.deltaY * 0.0025)));
        const worldX = (px - currentTransform.x) / currentTransform.k;
        const worldY = (py - currentTransform.y) / currentTransform.k;
        const nextTransform = d32.zoomIdentity.translate(px - worldX * nextScale, py - worldY * nextScale).scale(nextScale);
        svg.call(zoom2.transform, nextTransform);
        return;
      }
      const nextTransform = d32.zoomIdentity.translate(currentTransform.x - event.deltaX, currentTransform.y - event.deltaY).scale(currentTransform.k);
      svg.call(zoom2.transform, nextTransform);
    }, { passive: false });
    svg.on("click.nodeInfo", () => setNodeInfoRef.current(null));`;
const zoomWheelPatched = `    svg.call(zoom2);
    zoomRef.current = zoom2;
    svg.on("wheel.phgoViewport", (event) => {
      event.preventDefault();
      const currentTransform = transformRef.current ?? d32.zoomIdentity;
      if (event.ctrlKey || event.metaKey) {
        const rect = svgRef.current?.getBoundingClientRect();
        if (!rect) return;
        const px = event.clientX - rect.left;
        const py = event.clientY - rect.top;
        const unit = event.deltaMode === 1 ? 0.05 : event.deltaMode ? 1 : 0.002;
        const nextScale = Math.max(0.25, Math.min(4, currentTransform.k * Math.pow(2, -event.deltaY * unit * 10)));
        const worldX = (px - currentTransform.x) / currentTransform.k;
        const worldY = (py - currentTransform.y) / currentTransform.k;
        const nextTransform = d32.zoomIdentity.translate(px - worldX * nextScale, py - worldY * nextScale).scale(nextScale);
        svg.call(zoom2.transform, nextTransform);
        return;
      }
      const nextTransform = d32.zoomIdentity.translate(currentTransform.x - event.deltaX, currentTransform.y - event.deltaY).scale(currentTransform.k);
      svg.call(zoom2.transform, nextTransform);
    }, { passive: false });
    svg.on("click.nodeInfo", () => setNodeInfoRef.current(null));`;
const zoomScaleHandlerAnchor = `  const handleResizeDown = useCallback((e) => {
    const startY = e.clientY, startH = containerHRef.current;
    document.body.style.cursor = "ns-resize";
    document.body.style.userSelect = "none";
    const onMove = (ev) => setContainerH(Math.max(200, Math.min(1400, startH + ev.clientY - startY)));
    const onUp = () => {
      document.body.style.cursor = "";
      document.body.style.userSelect = "";
      window.removeEventListener("mousemove", onMove);
      window.removeEventListener("mouseup", onUp);
    };
    window.addEventListener("mousemove", onMove);
    window.addEventListener("mouseup", onUp);
    e.preventDefault();
  }, []);
  const drawAlnCanvas = useCallback(() => {`;
const zoomScaleHandlerPatched = `  const handleResizeDown = useCallback((e) => {
    const startY = e.clientY, startH = containerHRef.current;
    document.body.style.cursor = "ns-resize";
    document.body.style.userSelect = "none";
    const onMove = (ev) => setContainerH(Math.max(200, Math.min(1400, startH + ev.clientY - startY)));
    const onUp = () => {
      document.body.style.cursor = "";
      document.body.style.userSelect = "";
      window.removeEventListener("mousemove", onMove);
      window.removeEventListener("mouseup", onUp);
    };
    window.addEventListener("mousemove", onMove);
    window.addEventListener("mouseup", onUp);
    e.preventDefault();
  }, []);
  const handleViewportScaleChange = useCallback((nextScale) => {
    if (!svgRef.current || !zoomRef.current || !wrapperRef.current) return;
    const clampedScale = Math.max(0.25, Math.min(4, nextScale));
    const currentTransform = transformRef.current ?? d32.zoomIdentity;
    const wrapperRect = wrapperRef.current.getBoundingClientRect();
    const centerX = wrapperRect.width / 2;
    const centerY = wrapperRect.height / 2;
    const worldX = (centerX - currentTransform.x) / currentTransform.k;
    const worldY = (centerY - currentTransform.y) / currentTransform.k;
    const nextTransform = d32.zoomIdentity.translate(centerX - worldX * clampedScale, centerY - worldY * clampedScale).scale(clampedScale);
    d32.select(svgRef.current).call(zoomRef.current.transform, nextTransform);
  }, []);
  const drawAlnCanvas = useCallback(() => {`;
const alignmentWheelOriginal = `  useEffect(() => {
    const els = [alnScrollWrapRef.current, alnLegendRef.current];
    const handler = (e) => {
      const el = e.currentTarget;
      if (Math.abs(e.deltaX) > Math.abs(e.deltaY)) return;
      e.preventDefault();
      el.scrollLeft += e.deltaY;
    };
    els.forEach((el) => el?.addEventListener("wheel", handler, { passive: false }));
    return () => els.forEach((el) => el?.removeEventListener("wheel", handler));
  }, [showAlignment]);`;
const alignmentWheelPatched = `  useEffect(() => {
    const els = [alnScrollWrapRef.current, alnLegendRef.current];
    let frame = null;
    const scheduleDraw = () => {
      if (frame != null) return;
      frame = requestAnimationFrame(() => {
        frame = null;
        drawAlnCanvasRef.current();
      });
    };
    const handler = (e) => {
      const el = e.currentTarget;
      if (Math.abs(e.deltaX) > Math.abs(e.deltaY)) return;
      e.preventDefault();
      el.scrollLeft += e.deltaY;
      scheduleDraw();
    };
    const scrollHandler = () => scheduleDraw();
    els.forEach((el) => el?.addEventListener("wheel", handler, { passive: false }));
    alnScrollWrapRef.current?.addEventListener("scroll", scrollHandler, { passive: true });
    return () => {
      els.forEach((el) => el?.removeEventListener("wheel", handler));
      alnScrollWrapRef.current?.removeEventListener("scroll", scrollHandler);
      if (frame != null) cancelAnimationFrame(frame);
    };
  }, [showAlignment]);`;
const alignmentResizeRedrawEffect = `  useEffect(() => {
    if (!showAlignment) return void 0;
    let frame = null;
    let timer = null;
    const redraw = () => {
      if (frame != null) return;
      frame = requestAnimationFrame(() => {
        frame = null;
        drawAlnCanvasRef.current();
      });
    };
    const redrawAfterSettled = () => {
      if (timer != null) window.clearTimeout(timer);
      timer = window.setTimeout(redraw, 90);
    };
    const targets = [
      alnScrollWrapRef.current,
      alnLabelsCanvasRef.current,
      alnCanvasRef.current?.parentElement,
      alnCanvasRef.current?.closest?.(".Reactree_alnPanel"),
      wrapperRef.current
    ].filter(Boolean);
    const observer = typeof ResizeObserver !== "undefined" ? new ResizeObserver(redrawAfterSettled) : null;
    targets.forEach((target) => observer?.observe(target));
    window.addEventListener("resize", redrawAfterSettled);
    window.addEventListener("phgo-alignment-resize", redrawAfterSettled);
    redrawAfterSettled();
    return () => {
      observer?.disconnect();
      window.removeEventListener("resize", redrawAfterSettled);
      window.removeEventListener("phgo-alignment-resize", redrawAfterSettled);
      if (timer != null) window.clearTimeout(timer);
      if (frame != null) cancelAnimationFrame(frame);
    };
  }, [showAlignment, parsedFasta, layout]);`;
const zoomPctAnchor = `  const hPct = (hScale - 0.3) / 3.7 * 100;
  const vPct = (vScale - 0.3) / 3.7 * 100;
  const fPct = (fontScale - 0.5) / 2 * 100;
  const swPct = (strokeWidth - 0.5) / 3.5 * 100;`;
const zoomPctPatched = `  const hPct = Math.max(0, Math.min(100, hScale / 4 * 100));
  const vPct = Math.max(0, Math.min(100, vScale / 4 * 100));
  const fPct = (fontScale - 0.5) / 2 * 100;
  const swPct = (strokeWidth - 0.5) / 3.5 * 100;
  const zoomPct = (Math.max(0.25, Math.min(4, viewportScale)) - 0.25) / 3.75 * 100;`;
const zoomSliderAnchor = [
  '        )',
  '      ] }),',
  '      collapsedNodes.size > 0 && /* @__PURE__ */ jsxs2(Fragment, { children: [',
].join('\n');
const zoomSliderBlock = `      /* @__PURE__ */ jsx2("div", { className: Reactree_default.divider }),
      /* @__PURE__ */ jsxs2("div", { className: Reactree_default.sliderGroup, style: { minWidth: 172 }, children: [
        /* @__PURE__ */ jsx2("span", { className: Reactree_default.sliderIcon, children: /* @__PURE__ */ jsxs2("svg", { width: "14", height: "14", viewBox: "0 0 14 14", fill: "none", children: [
          /* @__PURE__ */ jsx2("circle", { cx: "6", cy: "6", r: "4", stroke: "currentColor", strokeWidth: "1.4" }),
          /* @__PURE__ */ jsx2("path", { d: "M9 9l3 3", stroke: "currentColor", strokeWidth: "1.4", strokeLinecap: "round" }),
          /* @__PURE__ */ jsx2("path", { d: "M6 4v4M4 6h4", stroke: "currentColor", strokeWidth: "1.2", strokeLinecap: "round" })
        ] }) }),
        /* @__PURE__ */ jsx2("span", { className: Reactree_default.sliderLabel, children: "Zoom" }),
        /* @__PURE__ */ jsx2(
          "input",
          {
            className: Reactree_default.slider,
            type: "range",
            min: 0.25,
            max: 4,
            step: 0.05,
            value: viewportScale,
            style: { background: \`linear-gradient(to right, var(--clr-primary-a0) \${zoomPct}%, var(--clr-surface-a30) \${zoomPct}%)\` },
            onChange: (e) => handleViewportScaleChange(Number(e.target.value)),
            onDoubleClick: () => handleViewportScaleChange(1)
          }
        ),
        /* @__PURE__ */ jsxs2("button", { className: Reactree_default.sliderVal, onClick: () => handleViewportScaleChange(1), children: [
          viewportScale >= 10 ? viewportScale.toFixed(1) : viewportScale.toFixed(2),
          "\xD7"
        ] })
      ] })`;
const fontPickerBlock = `      /* @__PURE__ */ jsxs2("div", { className: "phgo-fontPicker", children: [
        /* @__PURE__ */ jsx2("span", { className: "phgo-fontPickerLabel", children: "Font" }),
        /* @__PURE__ */ jsx2("select", { className: "phgo-fontSelect", value: fontFamily, onChange: (e) => setFontFamily(e.target.value), title: "Choose label font", children: fontOptions.map((family) => /* @__PURE__ */ jsx2("option", { value: family, children: family === DEFAULT_TREE_FONT_FAMILY ? "Default (system-ui)" : family }, family)) })
      ] })`;
const zoomSliderInsertBlock = `${fontPickerBlock},
${zoomSliderBlock},`;
const zoomSliderPatched = [
  '        )',
  '      ] }),',
  zoomSliderInsertBlock,
  '      collapsedNodes.size > 0 && /* @__PURE__ */ jsxs2(Fragment, { children: [',
].join('\n');
const panHintOriginal = '            /* @__PURE__ */ jsx2("div", { className: Reactree_default.hint, children: "Scroll to zoom \\xB7 Drag to pan" }),';
const panHintPatched = '            /* @__PURE__ */ jsx2("div", { className: Reactree_default.hint, children: "Scroll to pan \\xB7 Pinch to zoom \\xB7 Drag to pan" }),';
const snapshotStateEffectAnchor = `  }, [treeData, history, layout, treeType, labelMode, alignLabels, truncateNames, showAlignment, containerH, hScale, vScale, fontScale, strokeWidth, colorOverrides, collapsedNodes, collapseLabels, activeColor, colorMode, rerootMode, flipMode, swapMode, searchOpen, searchQuery, searchMatchIdx, downloadOpen, downloadMenuPos, palettePos, cladeEditor, nodeInfo, theme]);
  useEffect(() => {
    onStateChange?.(snapshotState());
  }, [onStateChange, snapshotState]);`;
const snapshotStateEffectPatched = `  }, [treeData, history, layout, treeType, labelMode, alignLabels, truncateNames, showAlignment, containerH, hScale, vScale, fontScale, strokeWidth, colorOverrides, collapsedNodes, collapseLabels, activeColor, colorMode, rerootMode, flipMode, swapMode, searchOpen, searchQuery, searchMatchIdx, downloadOpen, downloadMenuPos, palettePos, cladeEditor, nodeInfo, theme]);
  const snapshotStateRef = useRef(snapshotState);
  snapshotStateRef.current = snapshotState;
  const onStateChangeRef = useRef(onStateChange);
  onStateChangeRef.current = onStateChange;
  const pendingViewportScaleRef = useRef(viewportScale);
  const viewportScaleFrameRef = useRef(null);
  const scheduleViewportScaleChange = useCallback((nextScale) => {
    pendingViewportScaleRef.current = nextScale;
    if (viewportScaleFrameRef.current != null) return;
    viewportScaleFrameRef.current = requestAnimationFrame(() => {
      viewportScaleFrameRef.current = null;
      const pendingScale = pendingViewportScaleRef.current;
      setViewportScale((currentScale) => Math.abs(currentScale - pendingScale) < 1e-4 ? currentScale : pendingScale);
    });
  }, []);
  const alnDrawFrameRef = useRef(null);
  const scheduleAlnCanvasDraw = useCallback(() => {
    if (alnDrawFrameRef.current != null) return;
    alnDrawFrameRef.current = requestAnimationFrame(() => {
      alnDrawFrameRef.current = null;
      drawAlnCanvasRef.current();
    });
  }, []);
  const stateChangeFrameRef = useRef(null);
  const scheduleStateChange = useCallback(() => {
    if (!onStateChangeRef.current) return;
    if (stateChangeFrameRef.current != null) cancelAnimationFrame(stateChangeFrameRef.current);
    stateChangeFrameRef.current = requestAnimationFrame(() => {
      stateChangeFrameRef.current = null;
      onStateChangeRef.current?.(snapshotStateRef.current());
    });
  }, []);
  useEffect(() => {
    return () => {
      if (viewportScaleFrameRef.current != null) cancelAnimationFrame(viewportScaleFrameRef.current);
      if (alnDrawFrameRef.current != null) cancelAnimationFrame(alnDrawFrameRef.current);
      if (stateChangeFrameRef.current != null) cancelAnimationFrame(stateChangeFrameRef.current);
    };
  }, []);
  useEffect(() => {
    onStateChange?.(snapshotState());
  }, [onStateChange, snapshotState]);`;
const snapshotStateBridgePrevious = `  const snapshotStateRef = useRef(snapshotState);
  snapshotStateRef.current = snapshotState;
  const onStateChangeRef = useRef(onStateChange);
  onStateChangeRef.current = onStateChange;
  const stateChangeFrameRef = useRef(null);
  const scheduleStateChange = useCallback(() => {
    if (!onStateChangeRef.current) return;
    if (stateChangeFrameRef.current != null) cancelAnimationFrame(stateChangeFrameRef.current);
    stateChangeFrameRef.current = requestAnimationFrame(() => {
      stateChangeFrameRef.current = null;
      onStateChangeRef.current?.(snapshotStateRef.current());
    });
  }, []);
  useEffect(() => {
    return () => {
      if (stateChangeFrameRef.current != null) cancelAnimationFrame(stateChangeFrameRef.current);
    };
  }, []);
  useEffect(() => {
    onStateChange?.(snapshotState());
  }, [onStateChange, snapshotState]);`;
const snapshotStateBridgePatched = `  const snapshotStateRef = useRef(snapshotState);
  snapshotStateRef.current = snapshotState;
  const onStateChangeRef = useRef(onStateChange);
  onStateChangeRef.current = onStateChange;
  const pendingViewportScaleRef = useRef(viewportScale);
  const viewportScaleFrameRef = useRef(null);
  const scheduleViewportScaleChange = useCallback((nextScale) => {
    pendingViewportScaleRef.current = nextScale;
    if (viewportScaleFrameRef.current != null) return;
    viewportScaleFrameRef.current = requestAnimationFrame(() => {
      viewportScaleFrameRef.current = null;
      const pendingScale = pendingViewportScaleRef.current;
      setViewportScale((currentScale) => Math.abs(currentScale - pendingScale) < 1e-4 ? currentScale : pendingScale);
    });
  }, []);
  const alnDrawFrameRef = useRef(null);
  const scheduleAlnCanvasDraw = useCallback(() => {
    if (alnDrawFrameRef.current != null) return;
    alnDrawFrameRef.current = requestAnimationFrame(() => {
      alnDrawFrameRef.current = null;
      drawAlnCanvasRef.current();
    });
  }, []);
  const stateChangeFrameRef = useRef(null);
  const scheduleStateChange = useCallback(() => {
    if (!onStateChangeRef.current) return;
    if (stateChangeFrameRef.current != null) cancelAnimationFrame(stateChangeFrameRef.current);
    stateChangeFrameRef.current = requestAnimationFrame(() => {
      stateChangeFrameRef.current = null;
      onStateChangeRef.current?.(snapshotStateRef.current());
    });
  }, []);
  useEffect(() => {
    return () => {
      if (viewportScaleFrameRef.current != null) cancelAnimationFrame(viewportScaleFrameRef.current);
      if (alnDrawFrameRef.current != null) cancelAnimationFrame(alnDrawFrameRef.current);
      if (stateChangeFrameRef.current != null) cancelAnimationFrame(stateChangeFrameRef.current);
    };
  }, []);
  useEffect(() => {
    onStateChange?.(snapshotState());
  }, [onStateChange, snapshotState]);`;

const alignmentTruncateOriginal = `            let text = displayTreeName(name);
            while (lc.measureText(text).width > maxW && text.length > 3)
              text = text.slice(0, -4) + "\\u2026";
            lc.fillText(text, NAME_X, top + cellH / 2 + 0.5);`;

const alignmentTruncateReplacement = `            const text = displayTreeName(name);
            lc.fillText(text, NAME_X, top + cellH / 2 + 0.5);`;

const treeLabelTruncateOriginal = `        const n = displayTreeName(d.data.name);
        return truncateNames && n.length > 30 ? n.slice(0, 28) + "\\u2026" : n;`;

const treeLabelTruncateOriginalLegacy = `        const n = (d.data.name || "").replace(/_/g, " ");
        return truncateNames && n.length > 30 ? n.slice(0, 28) + "\\u2026" : n;`;

const treeLabelTruncateReplacement = `        const n = displayTreeName(d.data.name);
        return n;`;

const truncateButtonOriginalCJS = `      hasTruncatableNames && /* @__PURE__ */ (0, import_jsx_runtime2.jsxs)(import_jsx_runtime2.Fragment, { children: [
        /* @__PURE__ */ (0, import_jsx_runtime2.jsx)("div", { className: Reactree_default.divider }),
        /* @__PURE__ */ (0, import_jsx_runtime2.jsxs)(
          "button",
          {
            className: \`\${Reactree_default.alignLabelsBtn} \${truncateNames ? Reactree_default.alignLabelsBtnActive : ""}\`,
            onClick: () => setTruncateNames((t) => !t),
            title: truncateNames ? "Names truncated \\u2014 click to show full names" : "Full names shown \\u2014 click to truncate",
            children: [
              /* @__PURE__ */ (0, import_jsx_runtime2.jsxs)("svg", { width: "13", height: "13", viewBox: "0 0 14 14", fill: "none", children: [
                /* @__PURE__ */ (0, import_jsx_runtime2.jsx)("path", { d: "M1 3.5h12", stroke: "currentColor", strokeWidth: "1.4", strokeLinecap: "round" }),
                /* @__PURE__ */ (0, import_jsx_runtime2.jsx)("path", { d: "M1 7h7", stroke: "currentColor", strokeWidth: "1.4", strokeLinecap: "round" }),
                /* @__PURE__ */ (0, import_jsx_runtime2.jsx)("circle", { cx: "9.5", cy: "7", r: "0.8", fill: "currentColor" }),
                /* @__PURE__ */ (0, import_jsx_runtime2.jsx)("circle", { cx: "11.5", cy: "7", r: "0.8", fill: "currentColor" }),
                /* @__PURE__ */ (0, import_jsx_runtime2.jsx)("circle", { cx: "13.5", cy: "7", r: "0.8", fill: "currentColor" }),
                /* @__PURE__ */ (0, import_jsx_runtime2.jsx)("path", { d: "M1 10.5h12", stroke: "currentColor", strokeWidth: "1.4", strokeLinecap: "round" })
              ] }),
              truncateNames ? "Truncate" : "Full names"
            ]
          }
        )
      ] }),`;

const truncateButtonOriginalMJS = `      hasTruncatableNames && /* @__PURE__ */ jsxs2(Fragment, { children: [
        /* @__PURE__ */ jsx2("div", { className: Reactree_default.divider }),
        /* @__PURE__ */ jsxs2(
          "button",
          {
            className: \`\${Reactree_default.alignLabelsBtn} \${truncateNames ? Reactree_default.alignLabelsBtnActive : ""}\`,
            onClick: () => setTruncateNames((t) => !t),
            title: truncateNames ? "Names truncated \\u2014 click to show full names" : "Full names shown \\u2014 click to truncate",
            children: [
              /* @__PURE__ */ jsxs2("svg", { width: "13", height: "13", viewBox: "0 0 14 14", fill: "none", children: [
                /* @__PURE__ */ jsx2("path", { d: "M1 3.5h12", stroke: "currentColor", strokeWidth: "1.4", strokeLinecap: "round" }),
                /* @__PURE__ */ jsx2("path", { d: "M1 7h7", stroke: "currentColor", strokeWidth: "1.4", strokeLinecap: "round" }),
                /* @__PURE__ */ jsx2("circle", { cx: "9.5", cy: "7", r: "0.8", fill: "currentColor" }),
                /* @__PURE__ */ jsx2("circle", { cx: "11.5", cy: "7", r: "0.8", fill: "currentColor" }),
                /* @__PURE__ */ jsx2("circle", { cx: "13.5", cy: "7", r: "0.8", fill: "currentColor" }),
                /* @__PURE__ */ jsx2("path", { d: "M1 10.5h12", stroke: "currentColor", strokeWidth: "1.4", strokeLinecap: "round" })
              ] }),
              truncateNames ? "Truncate" : "Full names"
            ]
          }
        )
      ] }),`;

function replaceRequired(text, original, replacement, description, file) {
  if (text.includes(replacement)) {
    console.log(`${description} already present: ${file}`);
    return text;
  }
  if (!text.includes(original)) {
    throw new Error(`${description} anchor not found in ${file}`);
  }
  console.log(`Patched ${description}: ${file}`);
  return text.replace(original, replacement);
}

function replaceAllRequired(text, original, replacement, description, file) {
  if (description === 'Reactree export size state') {
    const exportSizeStatePattern = /  const \[exportLongEdge, setExportLongEdge\] = useState\(numberOr\(initialSnapshot\.exportLongEdge, 4096\)\);\r?\n  const exportLongEdgeRef = useRef\(4096\);\r?\n  exportLongEdgeRef\.current = exportLongEdge;\r?\n/g;
    const count = (text.match(exportSizeStatePattern) ?? []).length;
    if (count > 0) {
      text = text.replace(exportSizeStatePattern, '');
      const anchor = /(^\s*const \[strokeWidth, setStrokeWidth\] = useState\(numberOr\(initialSnapshot\.strokeWidth, 1\.5\)\);\r?\n)/m;
      if (!anchor.test(text)) {
        throw new Error(`${description} anchor not found in ${file}`);
      }
      console.log(
        count > 1
          ? `${description} normalized duplicate: ${file}`
          : `${description} repositioning existing block: ${file}`,
      );
      return text.replace(anchor, `$1${exportSizeStateBlock}`);
    }
  }
  if (
    description === 'Reactree full-name default' &&
    text.includes('initialSnapshot.truncateNames')
  ) {
    console.log(`${description} already present: ${file}`);
    return text;
  }
  if (description === 'Reactree viewport-scale restore') {
    const single = '    setViewportScale(numberOr(snapshot.transform?.k, 1));';
    const doubled = `${single}\n${single}`;
    if (text.includes(doubled)) {
      console.log(`${description} normalized duplicate: ${file}`);
      return text.replace(doubled, single);
    }
    if (text.includes(single)) {
      console.log(`${description} already present: ${file}`);
      return text;
    }
  }
  if (description === 'Reactree viewport zoom percent') {
    const single = '  const zoomPct = (Math.max(0.25, Math.min(4, viewportScale)) - 0.25) / 3.75 * 100;';
    const oldSingle = '  const zoomPct = (Math.max(0.05, Math.min(14, viewportScale)) - 0.05) / 13.95 * 100;';
    const partialOldSingle = '  const zoomPct = (Math.max(0.25, Math.min(4, viewportScale)) - 0.05) / 13.95 * 100;';
    if ((text.match(/^\s*const zoomPct = .*$/gm) ?? []).length > 1 || text.includes(partialOldSingle)) {
      console.log(`${description} normalized zoomPct declarations: ${file}`);
      let keptZoomPct = false;
      text = text.split('\n').filter((line) => {
        if (!/^\s*const zoomPct = /.test(line)) return true;
        if (!keptZoomPct) {
          keptZoomPct = true;
          return true;
        }
        return false;
      }).join('\n').replace(/^\s*const zoomPct = .*$/m, single);
    }
    const doubled = `${single}\n${single}`;
    if (text.includes(doubled)) {
      console.log(`${description} normalized duplicate: ${file}`);
      return text.replace(doubled, single);
    }
    if (text.includes(oldSingle)) {
      console.log(`${description} upgraded range: ${file}`);
      return text.replaceAll(oldSingle, single);
    }
    if (text.includes(single)) {
      console.log(`${description} already present: ${file}`);
      return text;
    }
  }
  if (description === 'Reactree wheel filter override') {
    if (text.includes(zoomFilterRootFixed)) {
      console.log(`${description} already present: ${file}`);
      return text;
    }
  }
  if (description === 'Reactree zoom state sync') {
    if (text.includes(zoomEventStatePersistPatched) || text.includes(zoomEventStatePersistPrevious) || text.includes(zoomEventPatched)) {
      console.log(`${description} already present: ${file}`);
      return text;
    }
  }
  if (description === 'Reactree transform persistence') {
    if (text.includes(zoomEventStatePersistPatched)) {
      console.log(`${description} already present: ${file}`);
      return text;
    }
    if (text.includes(zoomEventStatePersistPrevious)) {
      console.log(`${description} upgraded old transform persistence: ${file}`);
      return text.split(zoomEventStatePersistPrevious).join(zoomEventStatePersistPatched);
    }
  }
  if (description === 'Reactree transform state bridge') {
    if (text.includes('const scheduleViewportScaleChange = useCallback((nextScale) => {')) {
      console.log(`${description} already present: ${file}`);
      return text;
    }
    if (text.includes(snapshotStateBridgePrevious)) {
      console.log(`${description} upgraded old bridge: ${file}`);
      return text.replace(snapshotStateBridgePrevious, snapshotStateBridgePatched);
    }
    if (text.includes(snapshotStateEffectAnchor)) {
      console.log(`${description} upgraded old bridge: ${file}`);
      return text.replace(snapshotStateEffectAnchor, snapshotStateEffectPatched);
    }
  }
  if (description === 'Reactree trackpad pan handler') {
    if (text.includes(zoomWheelPatched)) {
      console.log(`${description} already present: ${file}`);
      return text;
    }
    if (text.includes(zoomWheelPrevious)) {
      console.log(`${description} upgraded old wheel handler: ${file}`);
      return text.split(zoomWheelPrevious).join(zoomWheelPatched);
    }
  }
  if (description === 'Reactree viewport zoom slider') {
    const existingCount = text.split(zoomSliderBlock).length - 1;
    if (text.includes(zoomSliderInsertBlock)) {
      text = text.split(zoomSliderInsertBlock).join('');
    }
    if (existingCount > 0) {
      text = text.split(zoomSliderBlock).join('');
      text = text.replace(/^\s*,\s*$/gm, '');
      text = text.replace(/\n{3,}/g, '\n\n');
      console.log(
        existingCount > 1
          ? `${description} normalized duplicate: ${file}`
          : `${description} repositioning existing block: ${file}`,
      );
    }
    const collapsedAnchor = /\n(\s*collapsedNodes\.size > 0 && \/\* @__PURE__ \*\/ jsxs2\(Fragment, \{ children: \[)/;
    if (!collapsedAnchor.test(text)) {
      throw new Error(`${description} anchor not found in ${file}`);
    }
    console.log(`Patched ${description}: ${file}`);
    return text.replace(collapsedAnchor, `\n${zoomSliderInsertBlock}\n$1`);
  }
  if (description === 'Reactree viewport-scale state' && text.includes('const [viewportScale, setViewportScale]')) {
    console.log(`${description} already present: ${file}`);
    return text;
  }
  if (description === 'Reactree viewport-scale state') {
    const fallback = /(^\s*const \[strokeWidth, setStrokeWidth\] = useState\(numberOr\(initialSnapshot\.strokeWidth, 1\.5\)\);\r?\n)/m;
    if (fallback.test(text)) {
      console.log(`Patched ${description} with fallback anchor: ${file}`);
      return text.replace(
        fallback,
        `$1  const [viewportScale, setViewportScale] = useState(numberOr(initialSnapshot.transform?.k, 1));\n`,
      );
    }
    const plainFallback = /(^\s*const \[strokeWidth, setStrokeWidth\] = useState\(1\.5\);\r?\n)/m;
    if (plainFallback.test(text)) {
      console.log(`Patched ${description} with plain fallback anchor: ${file}`);
      return text.replace(plainFallback, `$1  const [viewportScale, setViewportScale] = useState(1);\n`);
    }
  }
  if (!text.includes(original)) {
    if (text.includes(replacement)) {
      console.log(`${description} already present: ${file}`);
      return text;
    }
    throw new Error(`${description} anchor not found in ${file}`);
  }
  console.log(`Patched ${description}: ${file}`);
  return text.split(original).join(replacement);
}

function replaceAllOptional(text, original, replacement, description, file) {
  try {
    return replaceAllRequired(text, original, replacement, description, file);
  } catch (error) {
    if (String(error?.message || '').includes('anchor not found')) {
      console.log(`${description} anchor missing; skipping patch: ${file}`);
      return text;
    }
    throw error;
  }
}

function ensureSnapshotStateBridge(text, file) {
  if (!text.includes('snapshotStateRef') && !text.includes('scheduleViewportScaleChange') && !text.includes('scheduleStateChange')) {
    return text;
  }
  const bridgeBlock = `  const snapshotStateRef = useRef(snapshotState);
  snapshotStateRef.current = snapshotState;
  const onStateChangeRef = useRef(onStateChange);
  onStateChangeRef.current = onStateChange;
  const pendingViewportScaleRef = useRef(viewportScale);
  const viewportScaleFrameRef = useRef(null);
  const scheduleViewportScaleChange = useCallback((nextScale) => {
    pendingViewportScaleRef.current = nextScale;
    if (viewportScaleFrameRef.current != null) return;
    viewportScaleFrameRef.current = requestAnimationFrame(() => {
      viewportScaleFrameRef.current = null;
      const pendingScale = pendingViewportScaleRef.current;
      setViewportScale((currentScale) => Math.abs(currentScale - pendingScale) < 1e-4 ? currentScale : pendingScale);
    });
  }, []);
  const alnDrawFrameRef = useRef(null);
  const scheduleAlnCanvasDraw = useCallback(() => {
    if (alnDrawFrameRef.current != null) return;
    alnDrawFrameRef.current = requestAnimationFrame(() => {
      alnDrawFrameRef.current = null;
      drawAlnCanvasRef.current();
    });
  }, []);
  const stateChangeFrameRef = useRef(null);
  const scheduleStateChange = useCallback(() => {
    if (!onStateChangeRef.current) return;
    if (stateChangeFrameRef.current != null) cancelAnimationFrame(stateChangeFrameRef.current);
    stateChangeFrameRef.current = requestAnimationFrame(() => {
      stateChangeFrameRef.current = null;
      onStateChangeRef.current?.(snapshotStateRef.current());
    });
  }, []);
  useEffect(() => {
    return () => {
      if (viewportScaleFrameRef.current != null) cancelAnimationFrame(viewportScaleFrameRef.current);
      if (alnDrawFrameRef.current != null) cancelAnimationFrame(alnDrawFrameRef.current);
      if (stateChangeFrameRef.current != null) cancelAnimationFrame(stateChangeFrameRef.current);
    };
  }, []);
`;
  const bridgePattern = /  const snapshotStateRef = useRef\(snapshotState\);\r?\n  snapshotStateRef\.current = snapshotState;\r?\n(?:  const onStateChangeRef = useRef\(onStateChange\);\r?\n  onStateChangeRef\.current = onStateChange;\r?\n)?(?:  const pendingViewportScaleRef = useRef\(viewportScale\);\r?\n  const viewportScaleFrameRef = useRef\(null\);\r?\n  const scheduleViewportScaleChange = useCallback\(\(nextScale\) => \{[\s\S]*?\r?\n  \}, \[\]\);\r?\n)?(?:  const alnDrawFrameRef = useRef\(null\);\r?\n  const scheduleAlnCanvasDraw = useCallback\(\(\) => \{[\s\S]*?\r?\n  \}, \[\]\);\r?\n)?(?:  const stateChangeFrameRef = useRef\(null\);\r?\n  const scheduleStateChange = useCallback\(\(\) => \{[\s\S]*?\r?\n  \}, \[\]\);\r?\n)?(?:  useEffect\(\(\) => \{\r?\n    return \(\) => \{[\s\S]*?\r?\n    \};\r?\n  \}, \[\]\);\r?\n)?/g;
  const count = (text.match(bridgePattern) ?? []).length;
  text = text.replace(bridgePattern, '');
  const effectAnchor = /(^\s*useEffect\(\(\) => \{\r?\n\s*onStateChange\?\.\(snapshotState\(\)\);\r?\n\s*\}, \[onStateChange, snapshotState\]\);\r?\n)/m;
  if (effectAnchor.test(text)) {
    console.log(
      count > 1
        ? `Reactree snapshot-state bridge normalized duplicate: ${file}`
        : count === 1
          ? `Reactree snapshot-state bridge repositioning existing block: ${file}`
          : `Patched Reactree snapshot-state bridge: ${file}`,
    );
    return text.replace(effectAnchor, `${bridgeBlock}$1`);
  }
  const snapshotAnchor = /(^\s*const snapshotState = useCallback\(\(\) => \{[\s\S]*?\r?\n\s*\}, \[[^\]]*\]\);\r?\n)/m;
  if (snapshotAnchor.test(text)) {
    console.log(`Patched Reactree snapshot-state bridge with snapshot fallback anchor: ${file}`);
    return text.replace(snapshotAnchor, `$1${bridgeBlock}`);
  }
  console.log(`Reactree snapshot-state bridge anchor missing; skipping patch: ${file}`);
  return text;
}

function patchAlignmentDrawCanvas(text, file) {
  if (text.includes('const ALN_PERF_VERSION = 2;')) {
    console.log(`Reactree alignment performance draw already present: ${file}`);
    return text;
  }
  const start = text.indexOf('  const drawAlnCanvas = useCallback(() => {');
  const endMarker = '  drawAlnCanvasRef.current = drawAlnCanvas;';
  const end = text.indexOf(endMarker, start);
  if (start === -1 || end === -1) {
    throw new Error(`Reactree alignment draw canvas anchors not found in ${file}`);
  }
  const optimized = `  const drawAlnCanvas = useCallback(() => {
    const ALN_PERF_VERSION = 2;
    void ALN_PERF_VERSION;
    const seqCanvas = alnCanvasRef.current;
    const lblCanvas = alnLabelsCanvasRef.current;
    const scrollWrap = alnScrollWrapRef.current;
    const spacer = scrollWrap?.querySelector(".Reactree_alnVirtualSpacer");
    const pfasta = parsedFastaRef.current;
    const showAln = showAlignmentRef.current;
    const isAA = isAminoAcidRef.current;
    const leaves = leafOrderRef.current;
    if (!showAln || !pfasta || leaves.length === 0) {
      [seqCanvas, lblCanvas].forEach((c) => {
        if (c) {
          const ctx = c.getContext("2d");
          ctx?.clearRect(0, 0, c.width, c.height);
        }
      });
      if (spacer) {
        spacer.style.width = "0px";
        spacer.style.height = "0px";
      }
      return;
    }
    const { seqMap, maxLen } = pfasta;
    const preparedRows = getPreparedAlignmentRows(seqMap, isAA);
    const canvasFontFamily = fontFamilyRef.current;
    const t = transformRef.current ?? d32.zoomIdentity;
    const k = t.k;
    const ty = t.y;
    const vSc = vScaleRef.current;
    const fSc = fontScaleRef.current;
    const LEGEND_H = 34;
    const H = Math.max(0, containerHRef.current - LEGEND_H);
    const isDark = document.documentElement.getAttribute("data-theme") === "dark";
    const CELL_W = 14;
    const AXIS_H = 24;
    const PHGO_DPR = Math.max(1, Math.min(1.5, window.devicePixelRatio || 1));
    const leafSpacing = 26 * vSc;
    const rows = leaves.map(({ name, treeX }, idx) => {
      const screenY = treeX * k + ty;
      const cellH = Math.max(3, leafSpacing * k * 0.76);
      const top = screenY - cellH / 2;
      const visible = top + cellH >= 0 && top <= H;
      return { name, idx, top, cellH, visible };
    });
    const visibleRows = rows.filter((row) => row.visible);
    if (lblCanvas) {
      const LBL_W = 172;
      const NUM_W = 24;
      const CHIP_W = 4;
      const NAME_X = NUM_W + 6 + CHIP_W + 7;
      const lblPixelW = Math.max(1, Math.round(LBL_W * PHGO_DPR));
      const lblPixelH = Math.max(1, Math.round(H * PHGO_DPR));
      if (lblCanvas.width !== lblPixelW) lblCanvas.width = lblPixelW;
      if (lblCanvas.height !== lblPixelH) lblCanvas.height = lblPixelH;
      lblCanvas.style.width = \`\${LBL_W}px\`;
      lblCanvas.style.height = \`\${H}px\`;
      const lc = lblCanvas.getContext("2d", { alpha: false });
      if (lc) {
        lc.setTransform(PHGO_DPR, 0, 0, PHGO_DPR, 0, 0);
        const bg02 = isDark ? "#1e293b" : "#ffffff";
        const stripe = isDark ? "rgba(15,23,42,0.5)" : "rgba(241,245,249,0.75)";
        const sepClr = isDark ? "rgba(51,65,85,0.5)" : "rgba(226,232,240,0.8)";
        const numClr = isDark ? "#475569" : "#94a3b8";
        const txtClr = isDark ? "#cbd5e1" : "#1e293b";
        lc.fillStyle = bg02;
        lc.fillRect(0, 0, LBL_W, H);
        visibleRows.forEach(({ name, idx, top, cellH }) => {
          if (idx % 2 === 1) {
            lc.fillStyle = stripe;
            lc.fillRect(0, top, LBL_W, cellH);
          }
          if (cellH >= 5) {
            lc.fillStyle = sepClr;
            lc.fillRect(0, top + cellH - 0.5, LBL_W, 0.5);
          }
          if (cellH >= 8) {
            const numFs = Math.max(7, Math.min(16, cellH * 0.46 * fSc));
            lc.font = \`400 \${numFs}px \${canvasFontFamily}\`;
            lc.textAlign = "right";
            lc.textBaseline = "middle";
            lc.fillStyle = numClr;
            lc.fillText(String(idx + 1), NUM_W - 3, top + cellH / 2);
          }
          if (cellH >= 5) {
            lc.strokeStyle = isDark ? "rgba(255,255,255,0.04)" : "rgba(0,0,0,0.05)";
            lc.lineWidth = 1;
            lc.beginPath();
            lc.moveTo(NUM_W + 2, top + cellH * 0.18);
            lc.lineTo(NUM_W + 2, top + cellH * 0.82);
            lc.stroke();
          }
          if (cellH >= 6) {
            const chipH = Math.max(cellH * 0.52, 4);
            const chipY = top + (cellH - chipH) / 2;
            const chipX = NUM_W + 6;
            const hue = idx * 137.508 % 360;
            lc.fillStyle = \`hsl(\${hue.toFixed(0)},62%,52%)\`;
            if (lc.roundRect && chipH >= 5) {
              lc.beginPath();
              lc.roundRect(chipX, chipY, CHIP_W, chipH, 2);
              lc.fill();
            } else {
              lc.fillRect(chipX, chipY, CHIP_W, chipH);
            }
          }
          if (cellH >= 7) {
            const fs = Math.max(8, Math.min(20, cellH * 0.58 * fSc));
            lc.font = \`italic 500 \${fs}px \${canvasFontFamily}\`;
            lc.textAlign = "left";
            lc.textBaseline = "middle";
            lc.fillStyle = txtClr;
            lc.fillText(displayTreeName(name), NAME_X, top + cellH / 2 + 0.5);
          }
        });
        const hBg = isDark ? "rgba(15,23,42,0.97)" : "rgba(248,250,252,0.97)";
        lc.fillStyle = hBg;
        lc.fillRect(0, 0, LBL_W, AXIS_H);
        lc.fillStyle = isDark ? "#334155" : "#e2e8f0";
        lc.fillRect(0, AXIS_H - 1, LBL_W, 1);
        lc.font = \`600 8.5px \${canvasFontFamily}\`;
        lc.textAlign = "left";
        lc.textBaseline = "middle";
        lc.fillStyle = isDark ? "#64748b" : "#94a3b8";
        lc.fillText("ORGANISMS", NAME_X, AXIS_H / 2 + 0.5);
        lc.font = \`400 7.5px \${canvasFontFamily}\`;
        lc.textAlign = "right";
        lc.fillStyle = isDark ? "#334155" : "#cbd5e1";
        lc.fillText("#", NUM_W - 3, AXIS_H / 2 + 0.5);
        lc.fillStyle = isDark ? "#334155" : "#e2e8f0";
        lc.fillRect(LBL_W - 1, 0, 1, H);
      }
    }
    if (!seqCanvas || !scrollWrap) return;
    const totalW = maxLen * CELL_W;
    const viewportW = Math.max(1, Math.ceil(scrollWrap.clientWidth || Math.min(totalW, 800)));
    const scrollLeft = Math.max(0, Math.min(scrollWrap.scrollLeft || 0, Math.max(0, totalW - viewportW)));
    const visibleStartCol = Math.max(0, Math.floor(scrollLeft / CELL_W) - 1);
    const visibleEndCol = Math.min(maxLen, Math.ceil((scrollLeft + viewportW) / CELL_W) + 1);
    const paintX = visibleStartCol * CELL_W;
    const paintW = Math.max(1, viewportW + (scrollLeft - paintX) + CELL_W);
    if (spacer) {
      spacer.style.width = \`\${totalW}px\`;
      spacer.style.height = \`\${H}px\`;
    }
    const seqPixelW = Math.max(1, Math.round(paintW * PHGO_DPR));
    const seqPixelH = Math.max(1, Math.round(H * PHGO_DPR));
    if (seqCanvas.width !== seqPixelW) seqCanvas.width = seqPixelW;
    if (seqCanvas.height !== seqPixelH) seqCanvas.height = seqPixelH;
    seqCanvas.style.width = \`\${paintW}px\`;
    seqCanvas.style.height = \`\${H}px\`;
    seqCanvas.style.transform = \`translateX(\${paintX}px)\`;
    const sc = seqCanvas.getContext("2d", { alpha: false });
    if (!sc) return;
    sc.setTransform(PHGO_DPR, 0, 0, PHGO_DPR, -paintX * PHGO_DPR, 0);
    const bg0 = isDark ? "#1e293b" : "#ffffff";
    const bg1 = isDark ? "rgba(15,23,42,0.55)" : "rgba(241,245,249,0.7)";
    const sep = isDark ? "rgba(51,65,85,0.7)" : "rgba(226,232,240,0.9)";
    const gapColor = isDark ? "rgba(71,85,105,0.55)" : "rgba(148,163,184,0.6)";
    sc.fillStyle = bg0;
    sc.fillRect(paintX, 0, paintW, H);
    const colorRects = /* @__PURE__ */ new Map();
    const gapSegments = [];
    const letterOps = [];
    visibleRows.forEach(({ name, idx, top, cellH }) => {
      if (idx % 2 === 0) {
        sc.fillStyle = bg1;
        sc.fillRect(paintX, top, paintW, cellH);
      }
      if (cellH >= 6) {
        sc.fillStyle = sep;
        sc.fillRect(paintX, top + cellH - 0.5, paintW, 0.5);
      }
      const normName = (name || "").replace(/_/g, " ").toLowerCase().trim();
      let row = preparedRows.get(normName);
      if (!row) {
        for (const [key, candidate] of preparedRows) {
          if (key.includes(normName) || normName.includes(key)) {
            row = candidate;
            break;
          }
        }
      }
      if (!row) return;
      const endCol = Math.min(row.chars.length, visibleEndCol);
      const showLetters = cellH >= 11 && CELL_W >= 8;
      for (let i = visibleStartCol; i < endCol; i++) {
        const x = i * CELL_W;
        if (row.gaps[i]) {
          if (cellH >= 6) gapSegments.push([x + CELL_W * 0.22, top + cellH / 2, x + CELL_W * 0.78, top + cellH / 2, cellH >= 14 ? 1.5 : 1]);
          continue;
        }
        const color = row.colors[i];
        if (!color) continue;
        let rects = colorRects.get(color);
        if (!rects) {
          rects = [];
          colorRects.set(color, rects);
        }
        rects.push([x, top, CELL_W, cellH]);
        if (showLetters) letterOps.push([row.chars[i], x + CELL_W / 2, top + cellH / 2 + 0.5, cellH]);
      }
    });
    for (const [color, rects] of colorRects) {
      sc.fillStyle = color;
      for (const rect of rects) sc.fillRect(rect[0], rect[1], rect[2], rect[3]);
    }
    if (gapSegments.length) {
      sc.strokeStyle = gapColor;
      sc.lineCap = "round";
      let currentWidth = -1;
      sc.beginPath();
      for (const seg of gapSegments) {
        if (seg[4] !== currentWidth) {
          if (currentWidth !== -1) sc.stroke();
          currentWidth = seg[4];
          sc.lineWidth = currentWidth;
          sc.beginPath();
        }
        sc.moveTo(seg[0], seg[1]);
        sc.lineTo(seg[2], seg[3]);
      }
      sc.stroke();
    }
    if (letterOps.length) {
      const firstH = letterOps[0][3];
      const fs = Math.max(7, Math.min(18, firstH * 0.62 * fSc));
      sc.font = \`700 \${fs}px ui-monospace,'SF Mono',Menlo,monospace\`;
      sc.textAlign = "center";
      sc.textBaseline = "middle";
      sc.fillStyle = "rgba(255,255,255,0.95)";
      for (const op of letterOps) sc.fillText(op[0], op[1], op[2]);
    }
    sc.fillStyle = isDark ? "rgba(15,23,42,0.92)" : "rgba(248,250,252,0.95)";
    sc.fillRect(paintX, 0, paintW, AXIS_H);
    sc.fillStyle = isDark ? "#334155" : "#e2e8f0";
    sc.fillRect(paintX, AXIS_H - 1, paintW, 1);
    for (let i = visibleStartCol; i < visibleEndCol; i++) {
      const x = i * CELL_W;
      if (i % 10 === 0) {
        sc.fillStyle = isDark ? "#475569" : "#cbd5e1";
        sc.fillRect(x, AXIS_H - 5, 1, 5);
        sc.fillStyle = isDark ? "#94a3b8" : "#64748b";
        sc.font = \`8.5px \${canvasFontFamily}\`;
        sc.textAlign = "left";
        sc.textBaseline = "middle";
        sc.fillText(String(i + 1), x + 2, AXIS_H / 2 + 1);
      } else if (i % 5 === 0) {
        sc.fillStyle = isDark ? "#334155" : "#e2e8f0";
        sc.fillRect(x, AXIS_H - 3, 1, 3);
      }
    }
  }, [getPreparedAlignmentRows]);
`;
  console.log(`Patched Reactree alignment performance draw: ${file}`);
  return `${text.slice(0, start)}${optimized}${text.slice(end)}`;
}

for (const file of files) {
  let text = readFileSync(file, 'utf8').replace(/\r\n/g, '\n');
  if (!text.includes('PHGO_DPR')) {
    if (!text.includes(dprAnchor) || !text.includes(labelOriginal) || !text.includes(seqOriginal)) {
      throw new Error(`reactreejs alignment canvas patch anchors not found in ${file}`);
    }
    text = text
      .replace(dprAnchor, dprPatch)
      .replace(labelOriginal, labelPatched)
      .replace(seqOriginal, seqPatched);
    writeFileSync(file, text);
    console.log(`Patched Reactree alignment canvas DPI scaling: ${file}`);
  } else {
    console.log(`Reactree alignment canvas DPI patch already present: ${file}`);
  }

  text = readFileSync(file, 'utf8').replace(/\r\n/g, '\n');
  const heightPatchNeedles = [
    'useEffect(() => {\n    setContainerH(defaultHeight);\n  }, [defaultHeight]);',
    '(0, import_react.useEffect)(() => {\n    setContainerH(defaultHeight);\n  }, [defaultHeight]);',
    'useEffect(() => {\n    if (initialSnapshot.containerH == null) setContainerH(defaultHeight);\n  }, [defaultHeight, initialSnapshot.containerH]);',
    '(0, import_react.useEffect)(() => {\n    if (initialSnapshot.containerH == null) setContainerH(defaultHeight);\n  }, [defaultHeight, initialSnapshot.containerH]);'
  ];
  if (!heightPatchNeedles.some((needle) => text.includes(needle))) {
    const heightAnchors = [
      '  const [containerH, setContainerH] = useState(defaultHeight);',
      '  const [containerH, setContainerH] = (0, import_react.useState)(defaultHeight);'
    ];
    const heightAnchor = heightAnchors.find((anchor) => text.includes(anchor));
    if (!heightAnchor) {
      throw new Error(`reactreejs height sync anchor not found in ${file}`);
    }
    const heightPatch = `${heightAnchor}\n  useEffect(() => {\n    setContainerH(defaultHeight);\n  }, [defaultHeight]);`;
    const heightPatchCjs = `${heightAnchor}\n  (0, import_react.useEffect)(() => {\n    setContainerH(defaultHeight);\n  }, [defaultHeight]);`;
    text = text.replace(heightAnchor, file.endsWith('.js') ? heightPatchCjs : heightPatch);
    writeFileSync(file, text);
    console.log(`Patched Reactree defaultHeight sync: ${file}`);
  } else {
    console.log(`Reactree defaultHeight sync already present: ${file}`);
  }

  text = readFileSync(file, 'utf8');
  text = replaceRequired(text, parseOriginal, parsePatched, 'Reactree quoted Newick parser', file);
  text = replaceRequired(text, toNewickOriginal, toNewickPatched, 'Reactree quoted Newick exporter', file);
  if (text.includes('function displayTreeName(name)')) {
    console.log(`Reactree display-name helper already present: ${file}`);
  } else {
    text = replaceRequired(text, displayNameAnchor, displayNamePatched, 'Reactree display-name helper', file);
  }
  text = replaceAllRequired(text, triggerDownloadPatched, triggerDownloadBridgePatched, 'Reactree save-picker bridge', file);
  const isCJS = file.endsWith('.js');
  text = replaceAllRequired(
    text,
    isCJS ? labelModeGuardOriginalCJS : labelModeGuardOriginalMJS,
    isCJS ? labelModeGuardPatchedCJS : labelModeGuardPatchedMJS,
    'Reactree label-mode guard',
    file,
  );
  text = replaceAllRequired(
    text,
    isCJS
      ? '(0, import_react.useEffect)(() => {\n    if (initialSnapshot.containerH == null) setContainerH(defaultHeight);\n  }, [defaultHeight, initialSnapshot.containerH]);'
      : 'useEffect(() => {\n    if (initialSnapshot.containerH == null) setContainerH(defaultHeight);\n  }, [defaultHeight, initialSnapshot.containerH]);',
    isCJS
      ? '(0, import_react.useEffect)(() => {\n    setContainerH(defaultHeight);\n  }, [defaultHeight]);'
      : 'useEffect(() => {\n    setContainerH(defaultHeight);\n  }, [defaultHeight]);',
    'Reactree viewport height sync',
    file,
  );
  text = replaceAllRequired(
    text,
    isCJS ? truncateStateOriginalCJS : truncateStateOriginalMJS,
    isCJS ? truncateStateReplacementCJS : truncateStateReplacementMJS,
    'Reactree full-name default',
    file,
  );
  if (!text.includes('const ALN_PERF_VERSION = 2;')) {
    if (text.includes(alignmentTruncateOriginal)) {
      text = replaceRequired(text, alignmentTruncateOriginal, alignmentTruncateReplacement, 'Reactree full alignment labels', file);
    } else {
      console.log(`Reactree full alignment labels anchor missing; skipping patch: ${file}`);
    }
  } else {
    console.log(`Reactree full alignment labels covered by performance draw: ${file}`);
  }
  if (text.includes(treeLabelTruncateOriginal)) {
    text = replaceRequired(text, treeLabelTruncateOriginal, treeLabelTruncateReplacement, 'Reactree full tree labels', file);
  } else if (text.includes(treeLabelTruncateOriginalLegacy)) {
    text = replaceRequired(text, treeLabelTruncateOriginalLegacy, treeLabelTruncateReplacement, 'Reactree full tree labels', file);
  } else {
    console.log(`Reactree full tree labels anchor missing; skipping patch: ${file}`);
  }
  text = replaceAllRequired(text, isCJS ? truncateButtonOriginalCJS : truncateButtonOriginalMJS, '', 'Reactree truncate toggle removal', file);
  text = replaceAllRequired(text, '(d.data.name || "").replace(/_/g, " ")', 'displayTreeName(d.data.name)', 'Reactree display label preservation', file);
  text = replaceAllRequired(text, '(d.target.data.name || "").replace(/_/g, " ")', 'displayTreeName(d.target.data.name)', 'Reactree target label preservation', file);
  text = replaceAllRequired(text, '(node.name || "").replace(/_/g, " ")', 'displayTreeName(node.name)', 'Reactree search label preservation', file);
  if (text.includes('function buildExportSVG(') && !text.includes('Number(exportLongEdge) || 4096')) {
    const upgraded = text.replace(
      /function buildExportSVG\(svgEl, fallbackWidth = 800, fallbackHeight = 600\) \{[\s\S]*?\n\}\nasync function triggerDownload/,
      `${exportSvgHelper}async function triggerDownload`,
    );
    if (upgraded === text) {
      throw new Error(`Reactree fixed-edge export SVG helper upgrade failed in ${file}`);
    }
    text = upgraded;
    console.log(`Upgraded Reactree fixed-edge export SVG helper: ${file}`);
  } else if (!text.includes('function buildExportSVG(')) {
    text = replaceRequired(
      text,
      'async function triggerDownload(url, filename, options) {\n',
      `${exportSvgHelper}async function triggerDownload(url, filename, options) {\n`,
      'Reactree fit-to-canvas export SVG helper',
      file,
    );
  } else {
    console.log(`Reactree fit-to-canvas export SVG helper already present: ${file}`);
  }
  const exportBridgeOriginal = isCJS ? handleDownloadOriginalCJS : handleDownloadOriginal;
  const exportBridgeLegacy = isCJS ? handleDownloadPatchedLegacyCJS : handleDownloadPatchedLegacy;
  const exportBridgePatched = isCJS ? handleDownloadPatchedCJS : handleDownloadPatched;
  if (text.includes(exportBridgePatched) && text.includes('exportLongEdgeRef.current')) {
    console.log(`Reactree export bridge already present: ${file}`);
  } else if (text.includes(exportBridgeLegacy)) {
    console.log(`Patched Reactree export bridge: ${file}`);
    text = text.split(exportBridgeLegacy).join(exportBridgePatched);
  } else if (text.includes('const handleDownload = useCallback(async (format) => {')) {
    const upgraded = text.replace(
      /  const handleDownload = useCallback\(async \(format\) => \{\n    if \(!svgRef\.current\) return;[\s\S]*?\n  \}, \[\]\);/,
      exportBridgePatched,
    );
    if (upgraded === text) {
      throw new Error(`Reactree fit-to-canvas export bridge upgrade failed in ${file}`);
    }
    text = upgraded;
    console.log(`Upgraded Reactree fit-to-canvas export bridge: ${file}`);
  } else {
    text = replaceAllRequired(
      text,
      exportBridgeOriginal,
      exportBridgePatched,
      'Reactree export bridge',
      file,
    );
  }
  text = replaceAllOptional(
    text,
    labelModeInitialPatched,
    labelModeInitialRootFixed,
    'Reactree label-mode root init',
    file,
  );
  text = replaceAllOptional(
    text,
    restoreTreeDataPatched,
    restoreTreeDataRootFixed,
    'Reactree restore tree-data root fix',
    file,
  );
  text = replaceAllOptional(
    text,
    restoreLabelModePatched,
    restoreLabelModeRootFixed,
    'Reactree restore label-mode root fix',
    file,
  );
  text = replaceAllOptional(
    text,
    restoreEffectDepsPatched,
    restoreEffectDepsRootFixed,
    'Reactree restore deps resize fix',
    file,
  );
  text = replaceAllOptional(
    text,
    viewportScaleStateAnchor,
    viewportScaleStatePatched,
    'Reactree viewport-scale state',
    file,
  );
  text = replaceAllOptional(
    text,
    viewportScaleRestoreAnchor,
    viewportScaleRestorePatched,
    'Reactree viewport-scale restore',
    file,
  );
  text = replaceAllOptional(
    text,
    zoomEventAnchor,
    zoomEventPatched,
    'Reactree zoom state sync',
    file,
  );
  text = replaceAllOptional(
    text,
    zoomEventPatched,
    zoomEventStatePersistPatched,
    'Reactree transform persistence',
    file,
  );
  text = text
    .replaceAll('scaleExtent([0.05, 14])', 'scaleExtent([0.25, 4])')
    .replaceAll('(Math.max(0.05, Math.min(14, viewportScale)) - 0.05) / 13.95 * 100', '(Math.max(0.25, Math.min(4, viewportScale)) - 0.25) / 3.75 * 100')
    .replaceAll('Math.max(0.05, Math.min(14,', 'Math.max(0.25, Math.min(4,')
    .replaceAll('min: 0.05,\n            max: 14,\n            step: 0.01,', 'min: 0.25,\n            max: 4,\n            step: 0.05,');
  text = replaceAllOptional(
    text,
    zoomFilterPatched,
    zoomFilterRootFixed,
    'Reactree wheel filter override',
    file,
  );
  text = replaceAllOptional(
    text,
    zoomWheelAnchor,
    zoomWheelPatched,
    'Reactree trackpad pan handler',
    file,
  );
  text = replaceAllOptional(
    text,
    zoomScaleHandlerAnchor,
    zoomScaleHandlerPatched,
    'Reactree viewport slider handler',
    file,
  );
  text = replaceAllOptional(
    text,
    zoomPctAnchor,
    zoomPctPatched,
    'Reactree viewport zoom percent',
    file,
  );
  text = text
    .replaceAll('const hPct = (hScale - 0.3) / 3.7 * 100;', 'const hPct = Math.max(0, Math.min(100, hScale / 4 * 100));')
    .replaceAll('const vPct = (vScale - 0.3) / 3.7 * 100;', 'const vPct = Math.max(0, Math.min(100, vScale / 4 * 100));');
  if (!text.includes('function prepareAlignmentRows(seqMap, isAA)')) {
    text = replaceRequired(
      text,
      'function detectIsAA(fasta) {\n',
      `${alignmentPerfHelper}function detectIsAA(fasta) {\n`,
      'Reactree alignment prepared rows helper',
      file,
    );
  } else {
    console.log(`Reactree alignment prepared rows helper already present: ${file}`);
  }
  if (!text.includes('alnVirtualSpacer: "Reactree_alnVirtualSpacer"')) {
    text = replaceRequired(
      text,
      '  alnCanvas: "Reactree_alnCanvas",\n',
      '  alnCanvas: "Reactree_alnCanvas",\n  alnVirtualSpacer: "Reactree_alnVirtualSpacer",\n',
      'Reactree alignment virtual spacer class',
      file,
    );
  } else {
    console.log(`Reactree alignment virtual spacer class already present: ${file}`);
  }
  if (!text.includes('const alignmentPreparedRef = useRef({ source: null, isAA: null')) {
    text = replaceRequired(
      text,
      '  const drawAlnCanvasRef = useRef(() => {\n  });\n',
      `  const drawAlnCanvasRef = useRef(() => {\n  });\n${alignmentPerfBlock}`,
      'Reactree alignment prepared rows cache',
      file,
    );
  } else {
    console.log(`Reactree alignment prepared rows cache already present: ${file}`);
  }
  if (!text.includes('phgo-alignment-resize')) {
    const wheelAnchor = text.includes(alignmentWheelPatched) ? alignmentWheelPatched : alignmentWheelOriginal;
    text = replaceRequired(
      text,
      wheelAnchor,
      `${alignmentWheelPatched}
${alignmentResizeRedrawEffect}`,
      'Reactree alignment resize redraw observer',
      file,
    );
  } else {
    console.log(`Reactree alignment resize redraw observer already present: ${file}`);
  }
  if (!text.includes('className: Reactree_default.alnVirtualSpacer')) {
    text = replaceRequired(
      text,
      '              children: /* @__PURE__ */ jsx2("canvas", { ref: alnCanvasRef, className: Reactree_default.alnCanvas })\n',
      '              children: /* @__PURE__ */ jsxs2(Fragment, { children: [\n                /* @__PURE__ */ jsx2("div", { className: Reactree_default.alnVirtualSpacer }),\n                /* @__PURE__ */ jsx2("canvas", { ref: alnCanvasRef, className: Reactree_default.alnCanvas })\n              ] })\n',
      'Reactree alignment virtual spacer markup',
      file,
    );
  } else {
    console.log(`Reactree alignment virtual spacer markup already present: ${file}`);
  }
  text = patchAlignmentDrawCanvas(text, file);
  if (!text.includes('function OfficeRibbon({')) {
    text = replaceAllRequired(
      text,
      zoomSliderAnchor,
      zoomSliderPatched,
      'Reactree viewport zoom slider',
      file,
    );
  }
  text = replaceAllRequired(
    text,
    panHintOriginal,
    panHintPatched,
    'Reactree pan-zoom hint copy',
    file,
  );
  text = replaceAllOptional(
    text,
    snapshotStateEffectAnchor,
    snapshotStateEffectPatched,
    'Reactree transform state bridge',
    file,
  );
  text = ensureSnapshotStateBridge(text, file);
  for (const [original, replacement] of [
    ['.transition().duration(130)', '.transition().duration(0)'],
    ['.transition().duration(120)', '.transition().duration(0)'],
    ['.transition().duration(280)', '.transition().duration(0)'],
    ['.transition().duration(380)', '.transition().duration(0)'],
    ['.transition().duration(350)', '.transition().duration(0)'],
    ['.transition().duration(isLayoutChange || isFirstRender ? 320 : 200)', '.transition().duration(0)'],
  ]) {
    text = replaceAllRequired(text, original, replacement, 'Reactree D3 animation removal', file);
  }
  writeFileSync(file, text);
}

const viewerStateHelpers = `function cloneTreeData(node) {
  if (!node || typeof node !== "object") return node;
  const out = { name: String(node.name ?? "") };
  if (node.length != null && !isNaN(node.length)) out.length = Number(node.length);
  if (Array.isArray(node.children)) out.children = node.children.map(cloneTreeData);
  return out;
}
function mapToEntries(map) {
  return Array.from(map.entries()).map(([key, value]) => [key, value]);
}
function entriesToMap(entries) {
  return new Map(Array.isArray(entries) ? entries : []);
}
function setToValues(set) {
  return Array.from(set.values());
}
function valuesToSet(values) {
  return new Set(Array.isArray(values) ? values : []);
}
function numberOr(value, fallback) {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}
function boolOr(value, fallback) {
  return typeof value === "boolean" ? value : fallback;
}
function stringOr(value, fallback) {
  return typeof value === "string" ? value : fallback;
}
function transformToSnapshot(transform) {
  if (!transform) return null;
  return { x: numberOr(transform.x, 0), y: numberOr(transform.y, 0), k: numberOr(transform.k, 1) };
}
function transformFromSnapshot(transform) {
  if (!transform || typeof transform !== "object") return null;
  return d32.zoomIdentity.translate(numberOr(transform.x, 0), numberOr(transform.y, 0)).scale(numberOr(transform.k, 1));
}
function normalizeReactreeStateSnapshot(state) {
  return state && typeof state === "object" ? state : {};
}
function sanitizeLabelMode(labelMode, hasBootstraps, hasBranchLengths) {
  if (labelMode === "bootstrap" && hasBootstraps) return "bootstrap";
  if (labelMode === "branchlength" && hasBranchLengths) return "branchlength";
  if (labelMode === "none") return "none";
  return "none";
}`;

const viewerStateMenu = `            /* @__PURE__ */ jsx2("div", { className: Reactree_default.downloadMenuDivider }),
            /* @__PURE__ */ jsxs2(
              "button",
              {
                className: Reactree_default.downloadMenuItem,
                onClick: () => {
                  onViewerSnapshot?.(snapshotState());
                  setDownloadOpen(false);
                },
                children: [
                  /* @__PURE__ */ jsx2("span", { className: Reactree_default.downloadMenuIcon, children: /* @__PURE__ */ jsxs2("svg", { width: "12", height: "12", viewBox: "0 0 12 12", fill: "none", children: [
                    /* @__PURE__ */ jsx2("rect", { x: "1.2", y: "1.2", width: "9.6", height: "9.6", rx: "1.8", stroke: "currentColor", strokeWidth: "1.3" }),
                    /* @__PURE__ */ jsx2("path", { d: "M3.4 4h5.2M3.4 6h5.2M3.4 8h3.4", stroke: "currentColor", strokeWidth: "1.1", strokeLinecap: "round" })
                  ] }) }),
                  /* @__PURE__ */ jsx2("span", { className: Reactree_default.downloadMenuLabel, children: "PHgo Viewer Snapshot" }),
                  /* @__PURE__ */ jsx2("span", { className: Reactree_default.downloadMenuDesc, children: "Full state" })
                ]
              }
            ),`;

function patchReactreeViewerStateMJS(file) {
  let text = readFileSync(file, 'utf8').replace(/\r\n/g, '\n');
  if (text.includes('onViewerSnapshot') && text.includes('snapshotState') && !text.includes('function sanitizeLabelMode(')) {
    text = replaceRequired(
      text,
      `function normalizeReactreeStateSnapshot(state) {
  return state && typeof state === "object" ? state : {};
}`,
      `function normalizeReactreeStateSnapshot(state) {
  return state && typeof state === "object" ? state : {};
}
function sanitizeLabelMode(labelMode, hasBootstraps, hasBranchLengths) {
  if (labelMode === "bootstrap" && hasBootstraps) return "bootstrap";
  if (labelMode === "branchlength" && hasBranchLengths) return "branchlength";
  if (labelMode === "none") return "none";
  return "none";
}`,
      'Reactree sanitize-label helper',
      file,
    );
    writeFileSync(file, text);
    console.log(`Patched Reactree sanitize-label helper: ${file}`);
  }
  if (text.includes('onViewerSnapshot') && text.includes('snapshotState')) {
    console.log(`Reactree viewer-state bridge already present: ${file}`);
    return;
  }
  if (text.includes(displayNamePatched)) {
    text = replaceRequired(text, displayNamePatched, `${displayNamePatched}
${viewerStateHelpers}`, 'Reactree viewer-state helpers', file);
  } else if (text.includes('function displayTreeName(name) {\n  return String(name || "");\n}')) {
    text = replaceRequired(
      text,
      'function displayTreeName(name) {\n  return String(name || "");\n}',
      `function displayTreeName(name) {
  return String(name || "");
}
${viewerStateHelpers}`,
      'Reactree viewer-state helpers',
      file,
    );
  } else {
    text = replaceRequired(text, displayNamePatched, `${displayNamePatched}
${viewerStateHelpers}`, 'Reactree viewer-state helpers', file);
  }
  text = replaceRequired(
    text,
    'function Reactree({ newick, defaultHeight = 520, fasta }) {',
    'function Reactree({ newick, defaultHeight = 520, fasta, initialState, onStateChange, onViewerSnapshot, showPhgoCoords = false, setShowPhgoCoords }) {',
    'Reactree viewer-state props',
    file,
  );
  text = replaceRequired(
    text,
    `  const [{ treeData, history }, dispatch] = useReducer(
    treeReducer,
    null,
    () => ({ treeData: parseNewick(newick), history: [] })
  );`,
    `  const initialSnapshot = normalizeReactreeStateSnapshot(initialState);
  const [{ treeData, history }, dispatch] = useReducer(
    treeReducer,
    null,
    () => ({ treeData: initialSnapshot.treeData ? cloneTreeData(initialSnapshot.treeData) : parseNewick(newick), history: Array.isArray(initialSnapshot.history) ? initialSnapshot.history.map(cloneTreeData) : [] })
  );`,
    'Reactree viewer-state reducer seed',
    file,
  );
  const replacements = [
    ['  const [rerootMode, setRerootMode] = useState(false);', '  const [rerootMode, setRerootMode] = useState(boolOr(initialSnapshot.rerootMode, false));'],
    ['  const [flipMode, setFlipMode] = useState(false);', '  const [flipMode, setFlipMode] = useState(boolOr(initialSnapshot.flipMode, false));'],
    ['  const [swapMode, setSwapMode] = useState(false);', '  const [swapMode, setSwapMode] = useState(boolOr(initialSnapshot.swapMode, false));'],
    ['  const [downloadOpen, setDownloadOpen] = useState(false);', '  const [downloadOpen, setDownloadOpen] = useState(boolOr(initialSnapshot.downloadOpen, false));'],
    ['  const [downloadMenuPos, setDownloadMenuPos] = useState(null);', '  const [downloadMenuPos, setDownloadMenuPos] = useState(initialSnapshot.downloadMenuPos ?? null);'],
    ['  const [colorMode, setColorMode] = useState(false);', '  const [colorMode, setColorMode] = useState(boolOr(initialSnapshot.colorMode, false));'],
    ['  const [activeColor, setActiveColor] = useState(PALETTE[0].hex);', '  const [activeColor, setActiveColor] = useState(stringOr(initialSnapshot.activeColor, PALETTE[0].hex));'],
    ['  const [colorOverrides, setColorOverrides] = useState(/* @__PURE__ */ new Map());', '  const [colorOverrides, setColorOverrides] = useState(entriesToMap(initialSnapshot.colorOverrides));'],
    ['  const [palettePos, setPalettePos] = useState(null);', '  const [palettePos, setPalettePos] = useState(initialSnapshot.palettePos ?? null);'],
    ['  const [layout, setLayout] = useState("rectangular");', '  const [layout, setLayout] = useState(stringOr(initialSnapshot.layout, "rectangular"));'],
    ['  const [treeType, setTreeType] = useState("phylogram");', '  const [treeType, setTreeType] = useState(stringOr(initialSnapshot.treeType, "phylogram"));'],
    ['  const [labelMode, setLabelMode] = useState("bootstrap");', '  const [labelMode, setLabelMode] = useState(stringOr(initialSnapshot.labelMode, "bootstrap"));'],
    ['  const [alignLabels, setAlignLabels] = useState(false);', '  const [alignLabels, setAlignLabels] = useState(boolOr(initialSnapshot.alignLabels, false));'],
    ['  const [truncateNames, setTruncateNames] = useState(false);', '  const [truncateNames, setTruncateNames] = useState(boolOr(initialSnapshot.truncateNames, false));'],
    ['  const [showAlignment, setShowAlignment] = useState(false);', '  const [showAlignment, setShowAlignment] = useState(boolOr(initialSnapshot.showAlignment, false));'],
    ['  const [collapsedNodes, setCollapsedNodes] = useState(/* @__PURE__ */ new Set());', '  const [collapsedNodes, setCollapsedNodes] = useState(valuesToSet(initialSnapshot.collapsedNodes));'],
    ['  const [collapseLabels, setCollapseLabels] = useState(/* @__PURE__ */ new Map());', '  const [collapseLabels, setCollapseLabels] = useState(entriesToMap(initialSnapshot.collapseLabels));'],
    ['  const [searchOpen, setSearchOpen] = useState(false);', '  const [searchOpen, setSearchOpen] = useState(boolOr(initialSnapshot.searchOpen, false));'],
    ['  const [searchQuery, setSearchQuery] = useState("");', '  const [searchQuery, setSearchQuery] = useState(stringOr(initialSnapshot.searchQuery, ""));'],
    ['  const [searchMatchIdx, setSearchMatchIdx] = useState(0);', '  const [searchMatchIdx, setSearchMatchIdx] = useState(numberOr(initialSnapshot.searchMatchIdx, 0));'],
    ['  const [containerH, setContainerH] = useState(defaultHeight);', '  const [containerH, setContainerH] = useState(numberOr(initialSnapshot.containerH, defaultHeight));'],
    ['  const [hScale, setHScale] = useState(1);', '  const [hScale, setHScale] = useState(numberOr(initialSnapshot.hScale, 1));'],
    ['  const [vScale, setVScale] = useState(1);', '  const [vScale, setVScale] = useState(numberOr(initialSnapshot.vScale, 1));'],
    ['  const [fontScale, setFontScale] = useState(1);', '  const [fontScale, setFontScale] = useState(numberOr(initialSnapshot.fontScale, 1));'],
    ['  const [strokeWidth, setStrokeWidth] = useState(1.5);', '  const [strokeWidth, setStrokeWidth] = useState(numberOr(initialSnapshot.strokeWidth, 1.5));'],
    ['      if (isLayoutChange || isFirstRender) {', '      if ((isLayoutChange || isFirstRender) && !transformRef.current) {'],
  ];
  for (const [original, replacement] of replacements) {
    text = replaceAllRequired(text, original, replacement, 'Reactree viewer-state field', file);
  }
  text = replaceRequired(
    text,
    `  const containerHRef = useRef(defaultHeight);
  containerHRef.current = containerH;
  useEffect(() => {
    dispatch({ type: "RESET", data: parseNewick(newick) });
    setRerootMode(false);
    setFlipMode(false);
    setSwapMode(false);
    setSwapFirst(null);
    setColorMode(false);
    setColorOverrides(/* @__PURE__ */ new Map());
    setCollapsedNodes(/* @__PURE__ */ new Set());
    setSearchQuery("");
    setSearchMatchIdx(0);
    setSearchOpen(false);
    setNodeInfo(null);
    transformRef.current = null;
  }, [newick]);`,
    `  const containerHRef = useRef(defaultHeight);
  containerHRef.current = containerH;
  const snapshotState = useCallback(() => {
    const cladeEditorDraft = cladeEditor?.sid ? document.getElementById(\`clade-input-\${cladeEditor.sid}\`)?.value ?? "" : "";
    return {
      schema_version: 3,
      treeData: cloneTreeData(treeData),
      history: history.map(cloneTreeData),
      currentNewick: toNewick(treeData) + ";",
      layout,
      treeType,
      labelMode,
      alignLabels,
      truncateNames,
      showAlignment,
      containerH,
      hScale,
      vScale,
      fontScale,
      strokeWidth,
      colorOverrides: mapToEntries(colorOverrides),
      collapsedNodes: setToValues(collapsedNodes),
      collapseLabels: mapToEntries(collapseLabels),
      activeColor,
      colorMode,
      rerootMode,
      flipMode,
      swapMode,
      searchOpen,
      searchQuery,
      searchMatchIdx,
      downloadOpen,
      downloadMenuPos,
      palettePos,
      cladeEditor: cladeEditor ? { ...cladeEditor, draft: cladeEditorDraft } : null,
      nodeInfo,
      transform: transformToSnapshot(transformRef.current),
      theme
    };
  }, [treeData, history, layout, treeType, labelMode, alignLabels, truncateNames, showAlignment, containerH, hScale, vScale, fontScale, strokeWidth, colorOverrides, collapsedNodes, collapseLabels, activeColor, colorMode, rerootMode, flipMode, swapMode, searchOpen, searchQuery, searchMatchIdx, downloadOpen, downloadMenuPos, palettePos, cladeEditor, nodeInfo, theme]);
  useEffect(() => {
    onStateChange?.(snapshotState());
  }, [onStateChange, snapshotState]);
  useEffect(() => {
    const snapshot = normalizeReactreeStateSnapshot(initialState);
    dispatch({ type: "RESET", data: snapshot.treeData ? cloneTreeData(snapshot.treeData) : parseNewick(newick) });
    setRerootMode(boolOr(snapshot.rerootMode, false));
    setFlipMode(boolOr(snapshot.flipMode, false));
    setSwapMode(boolOr(snapshot.swapMode, false));
    setSwapFirst(null);
    setColorMode(boolOr(snapshot.colorMode, false));
    setActiveColor(stringOr(snapshot.activeColor, PALETTE[0].hex));
    setColorOverrides(entriesToMap(snapshot.colorOverrides));
    setCollapsedNodes(valuesToSet(snapshot.collapsedNodes));
    setCollapseLabels(entriesToMap(snapshot.collapseLabels));
    setLayout(stringOr(snapshot.layout, "rectangular"));
    setTreeType(stringOr(snapshot.treeType, "phylogram"));
    setLabelMode(stringOr(snapshot.labelMode, "bootstrap"));
    setAlignLabels(boolOr(snapshot.alignLabels, false));
    setShowAlignment(boolOr(snapshot.showAlignment, false));
    setContainerH(numberOr(snapshot.containerH, defaultHeight));
    setHScale(numberOr(snapshot.hScale, 1));
    setVScale(numberOr(snapshot.vScale, 1));
    setFontScale(numberOr(snapshot.fontScale, 1));
    setStrokeWidth(numberOr(snapshot.strokeWidth, 1.5));
    setSearchQuery(stringOr(snapshot.searchQuery, ""));
    setSearchMatchIdx(numberOr(snapshot.searchMatchIdx, 0));
    setSearchOpen(boolOr(snapshot.searchOpen, false));
    setDownloadOpen(boolOr(snapshot.downloadOpen, false));
    setDownloadMenuPos(snapshot.downloadMenuPos ?? null);
    setPalettePos(snapshot.palettePos ?? null);
    setCladeEditor(snapshot.cladeEditor ?? null);
    setNodeInfo(snapshot.nodeInfo ?? null);
    transformRef.current = transformFromSnapshot(snapshot.transform);
  }, [newick, initialState]);`,
    'Reactree viewer-state snapshot effect',
    file,
  );
  text = replaceRequired(
    text,
    '                  defaultValue: existing?.label ?? "",',
    '                  defaultValue: cladeEditor.draft ?? existing?.label ?? "",',
    'Reactree clade editor draft restore',
    file,
  );
  text = replaceRequired(
    text,
    '            /* @__PURE__ */ jsx2("div", { className: Reactree_default.downloadMenuDivider }),',
    viewerStateMenu,
    'Reactree PGV export menu item',
    file,
  );
  writeFileSync(file, text);
  console.log(`Patched Reactree viewer-state bridge: ${file}`);
}

patchReactreeViewerStateMJS(join(packageRoot, 'dist', 'index.mjs'));

function patchReactreeViewerPolishMJS(file) {
  let text = readFileSync(file, 'utf8').replace(/\r\n/g, '\n');
  const originalText = text;
  const skippablePolish = new Set([
    'Reactree MEGA rectangular terminal label connector',
    'Reactree MEGA circular label gap',
    'Reactree SVG text native selection guard',
    'Reactree MEGA clade label font',
    'Reactree MEGA clade circle removal',
    'Reactree MEGA leader line color',
    'Reactree MEGA search circle removal',
    'Reactree MEGA rectangular scale bar',
    'Reactree MEGA scale font',
    'Reactree alignment redraw font dependency',
    'Reactree PHgo/MEGA toolbar toggle',
  ]);
  const okabeItoPalette = `var PALETTE = [
  { hex: "#0072b2", name: "Okabe-Ito Blue" },
  { hex: "#e69f00", name: "Okabe-Ito Orange" },
  { hex: "#009e73", name: "Okabe-Ito Bluish Green" },
  { hex: "#d55e00", name: "Okabe-Ito Vermillion" },
  { hex: "#cc79a7", name: "Okabe-Ito Reddish Purple" },
  { hex: "#56b4e9", name: "Okabe-Ito Sky Blue" },
  { hex: "#f0e442", name: "Okabe-Ito Yellow" },
  { hex: "#000000", name: "Okabe-Ito Black" }
];`;
  const wcagAAAPalette = `var PALETTE = [
  { hex: "#000000", name: "Black" },
  { hex: "#2f4f4f", name: "DarkSlateGray" },
  { hex: "#000080", name: "Navy" },
  { hex: "#00008b", name: "DarkBlue" },
  { hex: "#191970", name: "MidnightBlue" },
  { hex: "#0000cd", name: "MediumBlue" },
  { hex: "#0000ff", name: "Blue" },
  { hex: "#4b0082", name: "Indigo" },
  { hex: "#800080", name: "Purple" },
  { hex: "#483d8b", name: "DarkSlateBlue" },
  { hex: "#8b008b", name: "DarkMagenta" },
  { hex: "#8b0000", name: "DarkRed" },
  { hex: "#800000", name: "Maroon" },
  { hex: "#8b4513", name: "SaddleBrown" },
  { hex: "#a52a2a", name: "Brown" },
  { hex: "#006400", name: "DarkGreen" }
];`;
  const legacyMegaTreeFontLine = '    const treeFontFamily = isMegaStyle ? "\\"Times New ' + 'Roman\\", Times, serif" : "system-ui, -apple-system, sans-serif";';

  for (const [original, replacement] of [
    ['    const scrollWrap = alnScrollWrapRef.current;\n    const visibleStartCol = scrollWrap ? Math.max(0, Math.floor(scrollWrap.scrollLeft / CELL_W) - 2) : 0;\n    const visibleEndCol = scrollWrap ? Math.min(maxLen, Math.ceil((scrollWrap.scrollLeft + scrollWrap.clientWidth) / CELL_W) + 2) : maxLen;\n    const paintX = visibleStartCol * CELL_W;\n    const paintW = Math.max(CELL_W, (visibleEndCol - visibleStartCol) * CELL_W);\n', ''],
    ['    sc.fillRect(paintX, 0, paintW, H);', '    sc.fillRect(0, 0, totalW, H);'],
    ['        sc.fillRect(paintX, top, paintW, cellH);', '        sc.fillRect(0, top, totalW, cellH);'],
    ['        sc.fillRect(paintX, top + cellH - 0.5, paintW, 0.5);', '        sc.fillRect(0, top + cellH - 0.5, totalW, 0.5);'],
    ['      for (let i = visibleStartCol; i < Math.min(seq.length, visibleEndCol); i++) {', '      for (let i = 0; i < seq.length; i++) {'],
    ['    sc.fillRect(paintX, 0, paintW, AXIS_H);', '    sc.fillRect(0, 0, totalW, AXIS_H);'],
    ['    sc.fillRect(paintX, AXIS_H - 1, paintW, 1);', '    sc.fillRect(0, AXIS_H - 1, totalW, 1);'],
    ['    for (let i = visibleStartCol; i < visibleEndCol; i++) {', '    for (let i = 0; i < maxLen; i++) {'],
  ]) {
    text = text.split(original).join(replacement);
  }

  function replaceOnce(original, replacement, description) {
    if (description === 'Reactree export size state') {
      const exportSizeStatePattern = /  const \[exportLongEdge, setExportLongEdge\] = useState\(numberOr\(initialSnapshot\.exportLongEdge, 4096\)\);\r?\n  const exportLongEdgeRef = useRef\(4096\);\r?\n  exportLongEdgeRef\.current = exportLongEdge;\r?\n/g;
      const count = (text.match(exportSizeStatePattern) ?? []).length;
      if (count > 0) {
        text = text.replace(exportSizeStatePattern, '');
        const anchor = /(^\s*const \[strokeWidth, setStrokeWidth\] = useState\(numberOr\(initialSnapshot\.strokeWidth, 1\.5\)\);\r?\n)/m;
        if (!anchor.test(text)) {
          throw new Error(`${description} anchor not found in ${file}`);
        }
        text = text.replace(anchor, `$1${exportSizeStateBlock}`);
        console.log(
          count > 1
            ? `${description} normalized duplicate: ${file}`
            : `${description} repositioning existing block: ${file}`,
        );
        return;
      }
    }
    if (text.includes(replacement)) return;
    if (!text.includes(original)) {
      if (skippablePolish.has(description)) {
        console.log(`${description} anchor missing; skipping patch: ${file}`);
        return;
      }
      throw new Error(`${description} anchor not found in ${file}`);
    }
    text = text.replace(original, replacement);
    console.log(`Patched ${description}: ${file}`);
  }

  function replaceAll(original, replacement, description) {
    if (text.includes(replacement)) return;
    if (!text.includes(original)) {
      if (skippablePolish.has(description)) {
        console.log(`${description} anchor missing; skipping patch: ${file}`);
        return;
      }
      throw new Error(`${description} anchor not found in ${file}`);
    }
    text = text.split(original).join(replacement);
    console.log(`Patched ${description}: ${file}`);
  }

  text = text.split(wcagAAAPalette).join(okabeItoPalette);
  replaceAll(
    'schema_version: 1,\n      treeData',
    'schema_version: 3,\n      treeData',
    'Reactree viewer state schema v3',
  );
  replaceOnce(
    'var PALETTE = [\n  { hex: "#ef4444", name: "Red" },\n  { hex: "#f97316", name: "Orange" },\n  { hex: "#f59e0b", name: "Amber" },\n  { hex: "#84cc16", name: "Lime" },\n  { hex: "#10b981", name: "Emerald" },\n  { hex: "#14b8a6", name: "Teal" },\n  { hex: "#0ea5e9", name: "Sky" },\n  { hex: "#6366f1", name: "Indigo" },\n  { hex: "#8b5cf6", name: "Violet" },\n  { hex: "#ec4899", name: "Pink" },\n  { hex: "#64748b", name: "Slate" },\n  { hex: "#92400e", name: "Brown" }\n];',
    okabeItoPalette,
    'Reactree Okabe-Ito clade palette',
  );
  replaceOnce(
    '  const [layout, setLayout] = useState(stringOr(initialSnapshot.layout, "rectangular"));',
    '  const [layout, setLayout] = useState(stringOr(initialSnapshot.layout, "rectangular"));\n  const [renderStyle, setRenderStyle] = useState(stringOr(initialSnapshot.renderStyle, "phgo"));',
    'Reactree PHgo/MEGA style state',
  );
  replaceOnce(
    'function stringOr(value, fallback) {\n  return typeof value === "string" ? value : fallback;\n}\nfunction transformToSnapshot(transform) {',
    'function stringOr(value, fallback) {\n  return typeof value === "string" ? value : fallback;\n}\nconst DEFAULT_TREE_FONT_FAMILY = "system-ui, -apple-system, sans-serif";\nconst FALLBACK_FONT_FAMILIES = [\n  "Arial",\n  "Helvetica",\n  "Verdana",\n  "Tahoma",\n  "Trebuchet MS",\n  "Georgia",\n  "Times New Roman",\n  "Courier New",\n  "Monaco",\n  "Arial Narrow"\n];\nfunction normalizeFontFamilyName(value) {\n  return typeof value === "string" ? value.trim() : "";\n}\nfunction buildFontOptions(values) {\n  const seen = /* @__PURE__ */ new Set();\n  const options = [];\n  const push = (family) => {\n    const trimmed = normalizeFontFamilyName(family);\n    if (!trimmed) return;\n    const key = trimmed.toLowerCase();\n    if (seen.has(key)) return;\n    seen.add(key);\n    options.push(trimmed);\n  };\n  push(DEFAULT_TREE_FONT_FAMILY);\n  Array.isArray(values) && values.forEach(push);\n  FALLBACK_FONT_FAMILIES.forEach(push);\n  if (options.length > 1) {\n    const [defaultFamily, ...rest] = options;\n    rest.sort((left, right) => left.localeCompare(right, void 0, { sensitivity: "base" }));\n    return [defaultFamily, ...rest];\n  }\n  return options;\n}\nasync function loadLocalFontFamilies(currentFontFamily) {\n  const discovered = [currentFontFamily];\n  if (typeof window !== "undefined" && typeof window.queryLocalFonts === "function") {\n    try {\n      const fonts = await window.queryLocalFonts();\n      fonts.forEach((font) => discovered.push(font?.family));\n    } catch {\n    }\n  }\n  if (typeof document !== "undefined" && document.fonts && typeof document.fonts.forEach === "function") {\n    try {\n      document.fonts.forEach((fontFace) => discovered.push(fontFace?.family));\n    } catch {\n    }\n  }\n  return buildFontOptions(discovered);\n}\nfunction transformToSnapshot(transform) {',
    'Reactree font helper utilities',
  );
  replaceOnce(
    '  const [fontScale, setFontScale] = useState(numberOr(initialSnapshot.fontScale, 1));',
    '  const [fontScale, setFontScale] = useState(numberOr(initialSnapshot.fontScale, 1));\n  const [fontFamily, setFontFamily] = useState(stringOr(initialSnapshot.fontFamily, DEFAULT_TREE_FONT_FAMILY));\n  const [fontOptions, setFontOptions] = useState(() => buildFontOptions([stringOr(initialSnapshot.fontFamily, DEFAULT_TREE_FONT_FAMILY)]));',
    'Reactree font picker state',
  );
  replaceOnce(
    '  const fontScaleRef = useRef(1);\n  fontScaleRef.current = fontScale;\n  const strokeWidthRef = useRef(1.5);',
    '  const fontScaleRef = useRef(1);\n  fontScaleRef.current = fontScale;\n  const fontFamilyRef = useRef(DEFAULT_TREE_FONT_FAMILY);\n  fontFamilyRef.current = fontFamily;\n  const strokeWidthRef = useRef(1.5);',
    'Reactree font family ref',
  );
  replaceOnce(
    '  const [strokeWidth, setStrokeWidth] = useState(numberOr(initialSnapshot.strokeWidth, 1.5));',
    '  const [strokeWidth, setStrokeWidth] = useState(numberOr(initialSnapshot.strokeWidth, 1.5));\n' + exportSizeStateBlock.trimEnd(),
    'Reactree export size state',
  );
  replaceOnce(
    '  const containerHRef = useRef(defaultHeight);\n  containerHRef.current = containerH;\n  const snapshotState = useCallback(() => {',
    '  const containerHRef = useRef(defaultHeight);\n  containerHRef.current = containerH;\n  useEffect(() => {\n    let cancelled = false;\n    loadLocalFontFamilies(fontFamily).then((families) => {\n      if (cancelled) return;\n      setFontOptions((current) => {\n        const next = buildFontOptions([...current, ...families, fontFamily]);\n        if (next.length === current.length && next.every((family, idx) => family === current[idx])) {\n          return current;\n        }\n        return next;\n      });\n    }).catch(() => {\n    });\n    return () => {\n      cancelled = true;\n    };\n  }, [fontFamily]);\n  const snapshotState = useCallback(() => {',
    'Reactree font family loader',
  );
  replaceOnce(
    '      layout,\n      treeType,',
    '      layout,\n      renderStyle,\n      treeType,',
    'Reactree style snapshot field',
  );
  replaceOnce(
    '      fontScale,\n      strokeWidth,',
    '      fontScale,\n      fontFamily,\n      strokeWidth,',
    'Reactree font snapshot field',
  );
  replaceOnce(
    '      strokeWidth,\n      colorOverrides: mapToEntries(colorOverrides),',
    '      strokeWidth,\n      exportLongEdge,\n      colorOverrides: mapToEntries(colorOverrides),',
    'Reactree export size snapshot field',
  );
  const snapshotDepsOriginal = '  }, [treeData, history, layout, treeType, labelMode, alignLabels, truncateNames, showAlignment, containerH, hScale, vScale, fontScale, strokeWidth, colorOverrides, collapsedNodes, collapseLabels, activeColor, colorMode, rerootMode, flipMode, swapMode, searchOpen, searchQuery, searchMatchIdx, downloadOpen, downloadMenuPos, palettePos, cladeEditor, nodeInfo, theme]);';
  const snapshotDepsRenderStyle = '  }, [treeData, history, layout, renderStyle, treeType, labelMode, alignLabels, truncateNames, showAlignment, containerH, hScale, vScale, fontScale, strokeWidth, colorOverrides, collapsedNodes, collapseLabels, activeColor, colorMode, rerootMode, flipMode, swapMode, searchOpen, searchQuery, searchMatchIdx, downloadOpen, downloadMenuPos, palettePos, cladeEditor, nodeInfo, theme]);';
  const snapshotDepsRuntimeFont = '  }, [treeData, history, layout, renderStyle, treeType, labelMode, alignLabels, truncateNames, showAlignment, containerH, hScale, vScale, fontScale, fontFamily, strokeWidth, colorOverrides, collapsedNodes, collapseLabels, activeColor, colorMode, rerootMode, flipMode, swapMode, searchOpen, searchQuery, searchMatchIdx, downloadOpen, downloadMenuPos, palettePos, cladeEditor, nodeInfo, theme]);';
  const snapshotDepsExportSize = '  }, [treeData, history, layout, renderStyle, treeType, labelMode, alignLabels, truncateNames, showAlignment, containerH, hScale, vScale, fontScale, fontFamily, strokeWidth, exportLongEdge, colorOverrides, collapsedNodes, collapseLabels, activeColor, colorMode, rerootMode, flipMode, swapMode, searchOpen, searchQuery, searchMatchIdx, downloadOpen, downloadMenuPos, palettePos, cladeEditor, nodeInfo, theme]);';
  const snapshotDepsPHgoCoords = '  }, [treeData, history, layout, renderStyle, treeType, labelMode, alignLabels, truncateNames, showAlignment, showPhgoCoords, containerH, hScale, vScale, fontScale, fontFamily, strokeWidth, exportLongEdge, colorOverrides, collapsedNodes, collapseLabels, activeColor, colorMode, rerootMode, flipMode, swapMode, searchOpen, searchQuery, searchMatchIdx, downloadOpen, downloadMenuPos, palettePos, cladeEditor, nodeInfo, theme]);';
  const snapshotDepsPHgoCoordsDuplicate = '  }, [treeData, history, layout, renderStyle, treeType, labelMode, alignLabels, truncateNames, showAlignment, showPhgoCoords, showPhgoCoords, containerH, hScale, vScale, fontScale, fontFamily, strokeWidth, exportLongEdge, colorOverrides, collapsedNodes, collapseLabels, activeColor, colorMode, rerootMode, flipMode, swapMode, searchOpen, searchQuery, searchMatchIdx, downloadOpen, downloadMenuPos, palettePos, cladeEditor, nodeInfo, theme]);';
  if (text.includes(snapshotDepsPHgoCoordsDuplicate)) {
    replaceOnce(snapshotDepsPHgoCoordsDuplicate, snapshotDepsPHgoCoords, 'Reactree coordinate snapshot dependency dedupe');
  }
  if (!text.includes(snapshotDepsRuntimeFont) && !text.includes(snapshotDepsExportSize) && !text.includes(snapshotDepsPHgoCoords)) {
    if (text.includes(snapshotDepsOriginal)) {
      replaceOnce(snapshotDepsOriginal, snapshotDepsRenderStyle, 'Reactree style snapshot dependencies');
    }
    if (text.includes(snapshotDepsRenderStyle)) {
      replaceOnce(snapshotDepsRenderStyle, snapshotDepsRuntimeFont, 'Reactree font snapshot dependencies');
    }
    if (!text.includes(snapshotDepsRuntimeFont) && !text.includes(snapshotDepsExportSize) && !text.includes(snapshotDepsPHgoCoords)) {
      throw new Error(`Reactree snapshot dependencies anchor not found in ${file}`);
    }
  }
  if (text.includes(snapshotDepsRuntimeFont) && !text.includes(snapshotDepsExportSize) && !text.includes(snapshotDepsPHgoCoords)) {
    replaceOnce(snapshotDepsRuntimeFont, snapshotDepsExportSize, 'Reactree export size snapshot dependencies');
  }
  if (!text.includes(snapshotDepsExportSize) && !text.includes(snapshotDepsPHgoCoords)) {
    throw new Error(`Reactree export size snapshot dependencies anchor not found in ${file}`);
  }
  const snapshotRestoreOriginal = '    setLayout(stringOr(snapshot.layout, "rectangular"));\n    setTreeType(stringOr(snapshot.treeType, "phylogram"));';
  const snapshotRestoreRenderStyle = '    setLayout(stringOr(snapshot.layout, "rectangular"));\n    setRenderStyle(stringOr(snapshot.renderStyle, "phgo"));\n    setTreeType(stringOr(snapshot.treeType, "phylogram"));';
  const snapshotRestoreRuntimeFont = '    setLayout(stringOr(snapshot.layout, "rectangular"));\n    setRenderStyle(stringOr(snapshot.renderStyle, "phgo"));\n    setTreeType(stringOr(snapshot.treeType, "phylogram"));\n    setFontFamily(stringOr(snapshot.fontFamily, DEFAULT_TREE_FONT_FAMILY));';
  const snapshotRestoreExportSize = '    setLayout(stringOr(snapshot.layout, "rectangular"));\n    setRenderStyle(stringOr(snapshot.renderStyle, "phgo"));\n    setTreeType(stringOr(snapshot.treeType, "phylogram"));\n    setFontFamily(stringOr(snapshot.fontFamily, DEFAULT_TREE_FONT_FAMILY));\n    setExportLongEdge(numberOr(snapshot.exportLongEdge, 4096));';
  if (!text.includes(snapshotRestoreRuntimeFont) && !text.includes(snapshotRestoreExportSize)) {
    if (text.includes(snapshotRestoreOriginal)) {
      replaceOnce(snapshotRestoreOriginal, snapshotRestoreRenderStyle, 'Reactree style restore');
    }
    if (text.includes(snapshotRestoreRenderStyle)) {
      replaceOnce(snapshotRestoreRenderStyle, snapshotRestoreRuntimeFont, 'Reactree font restore');
    }
    if (!text.includes(snapshotRestoreRuntimeFont) && !text.includes(snapshotRestoreExportSize)) {
      throw new Error(`Reactree snapshot restore anchor not found in ${file}`);
    }
  }
  if (text.includes(snapshotRestoreRuntimeFont) && !text.includes(snapshotRestoreExportSize)) {
    replaceOnce(snapshotRestoreRuntimeFont, snapshotRestoreExportSize, 'Reactree export size restore');
  }
  if (!text.includes(snapshotRestoreExportSize)) {
    throw new Error(`Reactree export size restore anchor not found in ${file}`);
  }
  replaceOnce(
    '    const isCircular = layout === "circular";',
    '    const isCircular = layout === "circular";\n    const isMegaStyle = renderStyle === "mega";',
    'Reactree style render flag',
  );
  text = text.split(
    '    const baseColor = isMegaStyle ? "#000000" : isDark ? "#cbd5e1" : "#475569";\n    const scaleColor = isMegaStyle ? "#000000" : isDark ? "#94a3b8" : "#64748b";\n    const branchLenColor = isMegaStyle ? "#000000" : isDark ? "#22d3ee" : "#0891b2";\n' + legacyMegaTreeFontLine,
  ).join(
    '    const baseColor = isMegaStyle ? "#000000" : isDark ? "#cbd5e1" : "#475569";\n    const scaleColor = isMegaStyle ? "#000000" : isDark ? "#94a3b8" : "#64748b";\n    const branchLenColor = isMegaStyle ? "#000000" : isDark ? "#22d3ee" : "#0891b2";\n    const treeFontFamily = fontFamily;',
  );
  const megaPaletteOriginal = '    const baseColor = isDark ? "#cbd5e1" : "#475569";\n    const scaleColor = isDark ? "#94a3b8" : "#64748b";\n    const branchLenColor = isDark ? "#22d3ee" : "#0891b2";';
  const megaPaletteWithHardcodedFont = '    const baseColor = isMegaStyle ? "#000000" : isDark ? "#cbd5e1" : "#475569";\n    const scaleColor = isMegaStyle ? "#000000" : isDark ? "#94a3b8" : "#64748b";\n    const branchLenColor = isMegaStyle ? "#000000" : isDark ? "#22d3ee" : "#0891b2";\n    const treeFontFamily = "system-ui, -apple-system, sans-serif";';
  const megaPaletteWithRuntimeFont = '    const baseColor = isMegaStyle ? "#000000" : isDark ? "#cbd5e1" : "#475569";\n    const scaleColor = isMegaStyle ? "#000000" : isDark ? "#94a3b8" : "#64748b";\n    const branchLenColor = isMegaStyle ? "#000000" : isDark ? "#22d3ee" : "#0891b2";\n    const treeFontFamily = fontFamily;';
  if (!text.includes(megaPaletteWithRuntimeFont)) {
    if (text.includes(megaPaletteOriginal)) {
      replaceOnce(
        megaPaletteOriginal,
        megaPaletteWithRuntimeFont,
        'Reactree MEGA palette and font',
      );
    } else if (text.includes(megaPaletteWithHardcodedFont)) {
      replaceOnce(
        megaPaletteWithHardcodedFont,
        megaPaletteWithRuntimeFont,
        'Reactree MEGA palette runtime font upgrade',
      );
    } else {
      throw new Error(`Reactree MEGA palette and font anchor not found in ${file}`);
    }
  }
  if (!text.includes('    const treeFontFamily = fontFamily;') && text.includes(legacyMegaTreeFontLine)) {
    replaceOnce(
      legacyMegaTreeFontLine,
      '    const treeFontFamily = fontFamily;',
      'Reactree MEGA font reset',
    );
  }
  if (!text.includes('const ALN_PERF_VERSION = 2;')) {
    replaceOnce(
      '    const { seqMap, maxLen } = pfasta;\n    const t = transformRef.current ?? d32.zoomIdentity;\n',
      '    const { seqMap, maxLen } = pfasta;\n    const canvasFontFamily = fontFamilyRef.current;\n    const t = transformRef.current ?? d32.zoomIdentity;\n',
      'Reactree alignment canvas font family state',
    );
    replaceAll(
      '`400 ${numFs}px system-ui,-apple-system,sans-serif`',
      '`400 ${numFs}px ${canvasFontFamily}`',
      'Reactree alignment row number font',
    );
    replaceAll(
      '`italic 500 ${fs}px system-ui,-apple-system,sans-serif`',
      '`italic 500 ${fs}px ${canvasFontFamily}`',
      'Reactree alignment label font',
    );
    replaceAll(
      '`600 8.5px system-ui,-apple-system,sans-serif`',
      '`600 8.5px ${canvasFontFamily}`',
      'Reactree alignment header font',
    );
    replaceAll(
      '`400 7.5px system-ui,-apple-system,sans-serif`',
      '`400 7.5px ${canvasFontFamily}`',
      'Reactree alignment index font',
    );
    replaceAll(
      '"8.5px system-ui,-apple-system,sans-serif"',
      '`8.5px ${canvasFontFamily}`',
      'Reactree alignment axis font',
    );
  } else {
    console.log(`Reactree alignment font patches covered by performance draw: ${file}`);
  }
  replaceOnce(
    '    const treeFontFamily = "system-ui, -apple-system, sans-serif";',
    '    const treeFontFamily = fontFamily;',
    'Reactree tree font family state',
  );
  replaceAll(
    '.attr("stroke-opacity", 0.8)',
    '.attr("stroke-opacity", isMegaStyle ? 1 : 0.8)',
    'Reactree MEGA line opacity',
  );
  replaceAll(
    'linkPaths.filter((p) => p === d).attr("stroke-opacity", 0.8)',
    'linkPaths.filter((p) => p === d).attr("stroke-opacity", isMegaStyle ? 1 : 0.8)',
    'Reactree MEGA hover line opacity',
  );
  replaceAll(
    '      _addNodeVisuals(node);',
    '      if (!isMegaStyle) _addNodeVisuals(node);',
    'Reactree MEGA node circle removal',
  );
  replaceAll(
    '.attr("font-family", "system-ui, -apple-system, sans-serif").style("fill", "#ef4444")',
    '.attr("font-family", treeFontFamily).style("fill", isMegaStyle ? "#000000" : "#ef4444")',
    'Reactree MEGA bootstrap label style',
  );
  replaceAll(
    '.attr("font-family", "system-ui, -apple-system, sans-serif").style("fill", branchLenColor)',
    '.attr("font-family", treeFontFamily).style("fill", branchLenColor)',
    'Reactree MEGA branch label font',
  );
  replaceOnce(
    '        }).attr("text-anchor", "middle");\n      } else {\n        labels.attr("x", (d) => (d.parent.y - d.y) / 2).attr("y", -4).attr("text-anchor", "middle");\n      }',
    '        }).attr("text-anchor", isMegaStyle ? "end" : "middle");\n      } else {\n        labels.attr("x", isMegaStyle ? -6 : (d) => (d.parent.y - d.y) / 2).attr("y", -4).attr("text-anchor", isMegaStyle ? "end" : "middle");\n      }',
    'Reactree MEGA branch label alignment',
  );
  replaceAll(
    '.attr("font-family", "system-ui, -apple-system, sans-serif").attr("font-style", "italic")',
    '.attr("font-family", treeFontFamily).attr("font-style", isMegaStyle ? "normal" : "italic")',
    'Reactree MEGA leaf label font',
  );
  replaceOnce(
    '        (sel) => sel.attr("x", alignLabels ? (d) => innerWidth - d.y + 10 : 12).attr("dy", "0.35em").attr("font-size", fontSize).attr("font-family", treeFontFamily).attr("font-style", isMegaStyle ? "normal" : "italic")',
    '        (sel) => sel.attr("x", alignLabels ? (d) => innerWidth - d.y + (isMegaStyle ? 4 : 10) : isMegaStyle ? 4 : 12).attr("dy", "0.35em").attr("font-size", fontSize).attr("font-family", treeFontFamily).attr("font-style", isMegaStyle ? "normal" : "italic")',
    'Reactree MEGA rectangular label gap',
  );
  replaceOnce(
    '      d32.cluster().size([innerHeight, innerWidth])(root);\n      if (isPhylogram && maxCumLen > 0)',
    '      d32.cluster().size([innerHeight, innerWidth])(root);\n      if (isMegaStyle) {\n        const visibleLeaves = root.leaves().filter((d) => !d._children);\n        const step = visibleLeaves.length > 1 ? innerHeight / (visibleLeaves.length - 1) : 0;\n        visibleLeaves.forEach((leaf, idx) => {\n          leaf.x = visibleLeaves.length > 1 ? idx * step : innerHeight / 2;\n        });\n        root.eachAfter((d) => {\n          if (!d.children || !d.children.length) return;\n          const visibleChildren = d.children.filter((child) => Number.isFinite(child.x));\n          if (visibleChildren.length) d.x = visibleChildren.reduce((sum, child) => sum + child.x, 0) / visibleChildren.length;\n        });\n      }\n      if (isPhylogram && maxCumLen > 0)',
    'Reactree MEGA equal leaf spacing',
  );
  replaceAll(
    '11.5 / Math.sqrt(vScale) * fontScale',
    '11.5 / Math.sqrt(Math.max(vScale, 0.01)) * fontScale',
    'Reactree zero height font guard',
  );
  replaceAll(
    'const innerHeight = Math.max(root.leaves().length * leafSpacing, 60);',
    'const innerHeight = root.leaves().length * leafSpacing;',
    'Reactree height scale zero minimum',
  );
  replaceAll(
    'const radius = Math.max(maxR, 40);',
    'const radius = Math.max(maxR, 0);',
    'Reactree circular size zero minimum',
  );
  replaceOnce(
    '      const gap = 0.04 * Math.PI * (2 - vScale);\n      d32.cluster().size([2 * Math.PI - gap, radius])(root);\n      root.each((d) => {\n        d.x += gap / 2;\n      });',
    '      const gap = isMegaStyle ? 0 : 0.04 * Math.PI * (2 - vScale);\n      d32.cluster().size([2 * Math.PI - gap, radius])(root);\n      if (isMegaStyle) {\n        const visibleLeaves = root.leaves().filter((d) => !d._children);\n        const angularSpan = 2 * Math.PI - gap;\n        const step = visibleLeaves.length > 0 ? angularSpan / visibleLeaves.length : 0;\n        visibleLeaves.forEach((leaf, idx) => {\n          leaf.x = visibleLeaves.length > 1 ? idx * step : angularSpan / 2;\n        });\n        root.eachAfter((d) => {\n          if (!d.children || !d.children.length) return;\n          const visibleChildren = d.children.filter((child) => Number.isFinite(child.x));\n          if (!visibleChildren.length) return;\n          const sin = visibleChildren.reduce((sum, child) => sum + Math.sin(child.x), 0);\n          const cos = visibleChildren.reduce((sum, child) => sum + Math.cos(child.x), 0);\n          const angle = Math.atan2(sin, cos);\n          d.x = angle < 0 ? angle + 2 * Math.PI : angle;\n        });\n      }\n      root.each((d) => {\n        d.x += gap / 2;\n      });',
    'Reactree MEGA circular equal leaf spacing',
  );
  if (text.includes('const barLen = innerWidth > 0 ? maxCumLen * barPx / innerWidth : 0;')) {
    console.log(`Reactree zero width scale bar guard already present: ${file}`);
  } else if (text.includes('const barLen = maxCumLen * barPx / innerWidth;')) {
    replaceAll(
      'const barLen = maxCumLen * barPx / innerWidth;',
      'const barLen = innerWidth > 0 ? maxCumLen * barPx / innerWidth : 0;',
      'Reactree zero width scale bar guard',
    );
  } else {
    console.log(`Reactree zero width scale bar guard anchor missing; skipping patch: ${file}`);
  }
  replaceOnce(
    '      if (alignLabels) {\n        const leaderColor = isMegaStyle ? "#000000" : isDark ? "rgba(148,163,184,0.35)" : "rgba(100,116,139,0.3)";\n        node.filter((d) => !d.children).append("line").attr("class", "leader-line").attr("x1", 8).attr("x2", (d) => innerWidth - d.y).attr("y1", 0).attr("y2", 0).attr("stroke", leaderColor).attr("stroke-width", 1).attr("stroke-dasharray", "2 3").style("pointer-events", "none");\n      }\n      addLeafLabels(',
    '      if (alignLabels) {\n        const leaderColor = isMegaStyle ? "#000000" : isDark ? "rgba(148,163,184,0.35)" : "rgba(100,116,139,0.3)";\n        node.filter((d) => !d.children).append("line").attr("class", "leader-line").attr("x1", isMegaStyle ? 0 : 8).attr("x2", (d) => innerWidth - d.y + (isMegaStyle ? 4 : 0)).attr("y1", 0).attr("y2", 0).attr("stroke", leaderColor).attr("stroke-width", 1).attr("stroke-dasharray", isMegaStyle ? null : "2 3").style("pointer-events", "none");\n      }\n      if (isMegaStyle && !alignLabels) {\n        node.filter((d) => !d.children).append("line").attr("class", "terminal-label-line").attr("x1", 0).attr("x2", 4).attr("y1", 0).attr("y2", 0).attr("stroke", baseColor).attr("stroke-width", strokeWidth).attr("stroke-linecap", "square").style("pointer-events", "none");\n      }\n      addLeafLabels(',
    'Reactree MEGA rectangular terminal label connector',
  );
  replaceOnce(
    '          if (!alignLabels) return d.x < Math.PI ? 12 : -12;\n          return d.x < Math.PI ? radius - d.y + 12 : -(radius - d.y) - 12;',
    '          const labelPad = isMegaStyle ? 4 : 12;\n          if (!alignLabels) return d.x < Math.PI ? labelPad : -labelPad;\n          return d.x < Math.PI ? radius - d.y + labelPad : -(radius - d.y) - labelPad;',
    'Reactree MEGA circular label gap',
  );
  replaceAll(
    '.style("user-select", "none").text((d) => {',
    '.style("user-select", "none").style("-webkit-user-select", "none").attr("unselectable", "on").text((d) => {',
    'Reactree SVG text native selection guard',
  );
  replaceAll(
    '.attr("font-family", "system-ui, -apple-system, sans-serif").attr("font-style", cladeInfo?.label ? "normal" : "italic")',
    '.attr("font-family", treeFontFamily).attr("font-style", isMegaStyle ? "normal" : cladeInfo?.label ? "normal" : "italic")',
    'Reactree MEGA clade label font',
  );
  replaceAll(
    '        g.append("circle").attr("cx", 0).attr("cy", 0).attr("r", 3).attr("fill", triColor).attr("stroke", isDark ? "#1e293b" : "#fff").attr("stroke-width", 1.5).style("pointer-events", "none");',
    '        if (!isMegaStyle) g.append("circle").attr("cx", 0).attr("cy", 0).attr("r", 3).attr("fill", triColor).attr("stroke", isDark ? "#1e293b" : "#fff").attr("stroke-width", 1.5).style("pointer-events", "none");',
    'Reactree MEGA clade circle removal',
  );
  replaceAll(
    '        const leaderColor = isDark ? "rgba(148,163,184,0.35)" : "rgba(100,116,139,0.3)";',
    '        const leaderColor = isMegaStyle ? "#000000" : isDark ? "rgba(148,163,184,0.35)" : "rgba(100,116,139,0.3)";',
    'Reactree MEGA leader line color',
  );
  replaceOnce(
    '        d32.select(el).insert("circle", ":first-child").attr("class", "search-glow").attr("r", r).attr("fill", isCurrent ? "rgba(245,158,11,0.14)" : isDark ? "rgba(129,140,248,0.12)" : "rgba(99,102,241,0.09)").attr("stroke", color).attr("stroke-width", isCurrent ? 2.2 : 1.6).style("pointer-events", "none");\n        positions.push(getPos(d));',
    '        if (!isMegaStyle) d32.select(el).insert("circle", ":first-child").attr("class", "search-glow").attr("r", r).attr("fill", isCurrent ? "rgba(245,158,11,0.14)" : isDark ? "rgba(129,140,248,0.12)" : "rgba(99,102,241,0.09)").attr("stroke", color).attr("stroke-width", isCurrent ? 2.2 : 1.6).style("pointer-events", "none");\n        positions.push(getPos(d));',
    'Reactree MEGA search circle removal',
  );
  replaceOnce(
    '      if (isPhylogram && maxCumLen > 0) {\n        const scaleX = d32.scaleLinear().domain([0, maxCumLen]).range([0, innerWidth]);\n        const ticks = scaleX.ticks(8);\n        const sb = content.append("g").attr("transform", `translate(0,${innerHeight + 28})`);\n        sb.append("line").attr("x1", 0).attr("x2", innerWidth).attr("stroke", scaleColor).attr("stroke-width", 1).attr("stroke-linecap", "round");\n        ticks.forEach((t, i) => {\n          const x = scaleX(t), e = i === 0 || i === ticks.length - 1;\n          sb.append("line").attr("x1", x).attr("x2", x).attr("y2", e ? 8 : 5).attr("stroke", scaleColor).attr("stroke-width", e ? 1.4 : 1);\n          sb.append("text").attr("x", x).attr("y", 20).attr("text-anchor", i === 0 ? "start" : i === ticks.length - 1 ? "end" : "middle").attr("font-size", "9.5px").attr("font-family", "system-ui").style("fill", "var(--clr-text-muted,#64748b)").text(formatTick(t));\n        });\n        sb.append("text").attr("x", innerWidth / 2).attr("y", 36).attr("text-anchor", "middle").attr("font-size", "9px").attr("font-family", "system-ui").attr("letter-spacing", "0.04em").style("fill", "var(--clr-text-muted,#64748b)").text("substitutions / site");\n      }',
    '      if (isPhylogram && maxCumLen > 0) {\n        if (isMegaStyle) {\n          const barPx = Math.max(52, Math.min(120, innerWidth * 0.16));\n          const barLen = innerWidth > 0 ? maxCumLen * barPx / innerWidth : 0;\n          const sb = content.append("g").attr("transform", `translate(0,${innerHeight + 30})`);\n          sb.append("line").attr("x1", 0).attr("x2", barPx).attr("stroke", scaleColor).attr("stroke-width", 1.5).attr("stroke-linecap", "square");\n          [0, barPx].forEach((x) => sb.append("line").attr("x1", x).attr("x2", x).attr("y1", -5).attr("y2", 5).attr("stroke", scaleColor).attr("stroke-width", 1.5));\n          sb.append("text").attr("x", barPx / 2).attr("y", -8).attr("text-anchor", "middle").attr("font-size", "10px").attr("font-family", treeFontFamily).style("fill", scaleColor).text(formatTick(barLen));\n        } else {\n          const scaleX = d32.scaleLinear().domain([0, maxCumLen]).range([0, innerWidth]);\n          const ticks = scaleX.ticks(8);\n          const sb = content.append("g").attr("transform", `translate(0,${innerHeight + 28})`);\n          sb.append("line").attr("x1", 0).attr("x2", innerWidth).attr("stroke", scaleColor).attr("stroke-width", 1).attr("stroke-linecap", "round");\n          ticks.forEach((t, i) => {\n            const x = scaleX(t), e = i === 0 || i === ticks.length - 1;\n            sb.append("line").attr("x1", x).attr("x2", x).attr("y2", e ? 8 : 5).attr("stroke", scaleColor).attr("stroke-width", e ? 1.4 : 1);\n            sb.append("text").attr("x", x).attr("y", 20).attr("text-anchor", i === 0 ? "start" : i === ticks.length - 1 ? "end" : "middle").attr("font-size", "9.5px").attr("font-family", "system-ui").style("fill", "var(--clr-text-muted,#64748b)").text(formatTick(t));\n          });\n          sb.append("text").attr("x", innerWidth / 2).attr("y", 36).attr("text-anchor", "middle").attr("font-size", "9px").attr("font-family", "system-ui").attr("letter-spacing", "0.04em").style("fill", "var(--clr-text-muted,#64748b)").text("substitutions / site");\n        }\n      }',
    'Reactree MEGA rectangular scale bar',
  );
  replaceAll(
    '.attr("font-family", "system-ui").style("fill", "var(--clr-text-muted,#64748b)")',
    '.attr("font-family", treeFontFamily).style("fill", scaleColor)',
    'Reactree MEGA scale font',
  );
  text = text.split(
    '        sb.append("text").attr("x", barPx / 2).attr("y", 16).attr("text-anchor", "middle").attr("font-size", "9.5px").attr("font-family", "system-ui").style("fill", "var(--clr-text-muted,#64748b)").text(formatTick(barLen));',
  ).join(
    '        sb.append("text").attr("x", barPx / 2).attr("y", 16).attr("text-anchor", "middle").attr("font-size", "9.5px").attr("font-family", treeFontFamily).style("fill", scaleColor).text(formatTick(barLen));',
  );
  replaceOnce(
    '  }, [showAlignment, parsedFasta, containerH, vScale, fontScale, layout, drawAlnCanvas]);',
    '  }, [showAlignment, parsedFasta, containerH, vScale, fontScale, fontFamily, layout, drawAlnCanvas]);',
    'Reactree alignment redraw font dependency',
  );
  const renderDepsOriginal = '  }, [treeData, containerH, hScale, vScale, fontScale, strokeWidth, layout, treeType, colorOverrides, theme, labelMode, alignLabels, truncateNames, collapsedNodes, collapseLabels, searchQuery, searchMatchIdx, showAlignment]);';
  const renderDepsRenderStyle = '  }, [treeData, containerH, hScale, vScale, fontScale, strokeWidth, layout, renderStyle, treeType, colorOverrides, theme, labelMode, alignLabels, truncateNames, collapsedNodes, collapseLabels, searchQuery, searchMatchIdx, showAlignment]);';
  const renderDepsRuntimeFont = '  }, [treeData, containerH, hScale, vScale, fontScale, fontFamily, strokeWidth, layout, renderStyle, treeType, colorOverrides, theme, labelMode, alignLabels, truncateNames, collapsedNodes, collapseLabels, searchQuery, searchMatchIdx, showAlignment]);';
  const renderDepsPHGOLabels = '  }, [treeData, containerH, hScale, vScale, fontScale, fontFamily, strokeWidth, layout, renderStyle, treeType, colorOverrides, theme, labelMode, alignLabels, truncateNames, collapsedNodes, collapseLabels, searchQuery, searchMatchIdx, showAlignment, showPhgoCoords, labelMetadata]);';
  if (!text.includes(renderDepsRuntimeFont) && !text.includes(renderDepsPHGOLabels)) {
    if (text.includes(renderDepsOriginal)) {
      replaceOnce(renderDepsOriginal, renderDepsRenderStyle, 'Reactree style render dependencies');
    }
    if (text.includes(renderDepsRenderStyle)) {
      replaceOnce(renderDepsRenderStyle, renderDepsRuntimeFont, 'Reactree tree font render dependencies');
    }
    if (!text.includes(renderDepsRuntimeFont) && !text.includes(renderDepsPHGOLabels)) {
      throw new Error(`Reactree render dependencies anchor not found in ${file}`);
    }
  }
  if (!text.includes('function OfficeRibbon({')) {
    replaceOnce(
      '        }, title: "Circular", children: /* @__PURE__ */ jsx2(IconCirc, {}) })\n      ] }),\n      /* @__PURE__ */ jsx2("div", { className: Reactree_default.divider }),',
      '        }, title: "Circular", children: /* @__PURE__ */ jsx2(IconCirc, {}) })\n      ] }),\n      /* @__PURE__ */ jsx2("div", { className: Reactree_default.divider }),\n      /* @__PURE__ */ jsxs2("div", { className: Reactree_default.btnGroup, children: [\n        /* @__PURE__ */ jsx2("button", { className: `${Reactree_default.btnGroupItem} ${renderStyle === "phgo" ? Reactree_default.btnGroupActive : ""}`, onClick: () => setRenderStyle("phgo"), title: "PHgo style", children: "P" }),\n        /* @__PURE__ */ jsx2("button", { className: `${Reactree_default.btnGroupItem} ${renderStyle === "mega" ? Reactree_default.btnGroupActive : ""}`, onClick: () => setRenderStyle("mega"), title: "MEGA style", children: "M" })\n      ] }),\n      /* @__PURE__ */ jsx2("div", { className: Reactree_default.divider }),',
      'Reactree PHgo/MEGA toolbar toggle',
    );
  }

  if (!text.includes('const ALN_PERF_VERSION = 2;')) {
    throw new Error(`Reactree alignment performance draw missing in ${file}`);
  }
  if (text !== originalText) {
    writeFileSync(file, text);
    console.log(`Patched Reactree viewer polish: ${file}`);
  } else {
    console.log(`Reactree viewer polish already present: ${file}`);
  }
}

patchReactreeViewerPolishMJS(join(packageRoot, 'dist', 'index.mjs'));

function patchReactreePersistentEditModes(file) {
  let text = readFileSync(file, 'utf8').replace(/\r\n/g, '\n');
  const originalText = text;
  const replacements = [
    [
      '      setTooltip(null);\n      setRerootMode(false);\n      dispatch({ type: "REROOT", newData: buildRerooted(d, null) });',
      '      setTooltip(null);\n      dispatch({ type: "REROOT", newData: buildRerooted(d, null) });',
    ],
    [
      '      setTooltip(null);\n      setFlipMode(false);\n      dispatch({ type: "FLIP", newData: rebuildWithFlip(root, d) });',
      '      setTooltip(null);\n      dispatch({ type: "FLIP", newData: rebuildWithFlip(root, d) });',
    ],
    [
      '        setTooltip(null);\n        setSwapFirst(null);\n        setSwapMode(false);\n        dispatch({ type: "SWAP", newData: rebuildWithSwap(root, first, d) });',
      '        setTooltip(null);\n        setSwapFirst(null);\n        dispatch({ type: "SWAP", newData: rebuildWithSwap(root, first, d) });',
    ],
  ];
  for (const [original, replacement] of replacements) {
    if (text.includes(original)) {
      text = text.replace(original, replacement);
    } else if (!text.includes(replacement)) {
      throw new Error(`Reactree persistent edit-mode anchor not found in ${file}`);
    }
  }
  if (text !== originalText) {
    writeFileSync(file, text);
    console.log(`Patched Reactree persistent edit modes: ${file}`);
  } else {
    console.log(`Reactree persistent edit modes already present: ${file}`);
  }
}

patchReactreePersistentEditModes(join(packageRoot, 'dist', 'index.mjs'));

function patchReactreeOfficeRibbonMJS(file) {
  let text = readFileSync(file, 'utf8').replace(/\r\n/g, '\n');
  const originalText = text;
  text = text
    .replace(
      'function Reactree({ newick, defaultHeight = 520, fasta, initialState, onStateChange, onViewerSnapshot }) {',
      'function Reactree({ newick, defaultHeight = 520, fasta, initialState, onStateChange, onViewerSnapshot, showPhgoCoords = false, setShowPhgoCoords }) {',
    )
    .replace(
      '      showAlignment,\n      containerH,',
      '      showAlignment,\n      showPhgoCoords,\n      containerH,',
    )
    .replace(
      '  }, [treeData, history, layout, renderStyle, treeType, labelMode, alignLabels, truncateNames, showAlignment,',
      '  }, [treeData, history, layout, renderStyle, treeType, labelMode, alignLabels, truncateNames, showAlignment, showPhgoCoords,',
    )
    .replace(
      '      setShowAlignment,\n      alignLabels,\n      collapsedNodes,',
      '      setShowAlignment,\n      alignLabels,\n      showPhgoCoords,\n      setShowPhgoCoords,\n      collapsedNodes,',
    );
  text = text
    .replace(
      '  }, [treeData, history, layout, renderStyle, treeType, labelMode, alignLabels, truncateNames, showAlignment, showPhgoCoords, showPhgoCoords,',
      '  }, [treeData, history, layout, renderStyle, treeType, labelMode, alignLabels, truncateNames, showAlignment, showPhgoCoords,',
    )
    .replace(
      '      setShowAlignment,\n      alignLabels,\n      showPhgoCoords,\n      setShowPhgoCoords,\n      showPhgoCoords,\n      setShowPhgoCoords,\n      collapsedNodes,',
      '      setShowAlignment,\n      alignLabels,\n      showPhgoCoords,\n      setShowPhgoCoords,\n      collapsedNodes,',
    );
  const importAnchor = 'import { createPortal } from "react-dom";';
  text = text
    .replace(/^import \{[^}]*\} from "@fluentui\/react-components";\n?/m, '')
    .replace(/^import \{[^}]*\} from "@fluentui\/react-icons";\n?/m, '');
  const fluentImports = `import { Button, Dropdown, FluentProvider, Input, Menu, MenuButton, MenuDivider, MenuItem, MenuList, MenuPopover, MenuTrigger, Option, Tab, TabList, Tooltip, webLightTheme } from "@fluentui/react-components";
import { ArrowClockwiseRegular, ArrowDownloadRegular, ArrowUndoRegular, ArrowUpRegular, AutoFitHeightRegular, AutoFitWidthRegular, BranchForkRegular, ChevronDownRegular, DocumentSaveRegular, FolderOpenRegular, MoreHorizontalRegular, SearchRegular, SlideSizeRegular, TextFontRegular, TextFontSizeRegular, ZoomFitRegular, ZoomInRegular } from "@fluentui/react-icons";`;
  text = replaceRequired(
    text,
    importAnchor,
    `${importAnchor}\n${fluentImports}`,
    'Reactree Fluent imports',
    file,
  );

  const officeRibbonMarker = 'function OfficeRibbon({';
  const officeRibbonCode = `const OFFICE_TABS = [
  { key: "file", label: "File" },
  { key: "view", label: "View" },
  { key: "format", label: "Format" },
  { key: "edit", label: "Edit" }
];
const OFFICE_MORE_BUTTON_WIDTH = 76;
const OFFICE_SEARCH_BUTTON_WIDTH = 38;
const OFFICE_TAB_MORE_WIDTH = 38;
const OFFICE_FONT_LABELS = {
  [DEFAULT_TREE_FONT_FAMILY]: "Default",
  "system-ui, -apple-system, sans-serif": "Default"
};
function clampRibbonNumber(value, min, max) {
  const next = Number(value);
  if (!Number.isFinite(next)) return min;
  const lower = Number.isFinite(min) ? Math.max(min, next) : next;
  return Number.isFinite(max) ? Math.min(max, lower) : lower;
}
function measuredVisibleKeys(items, widths, available, moreWidth = OFFICE_MORE_BUTTON_WIDTH) {
  if (!items.length) return [];
  let used = 0;
  let lastGroupKey = null;
  const visible = [];
  for (const item of items) {
    const groupOverhead = item.groupKey ? (item.groupKey === lastGroupKey ? 4 : 17) : 0;
    const width = Math.ceil(widths[item.key] ?? 0) + groupOverhead;
    if (width <= 0 || used + width <= available) {
      visible.push({ key: item.key, width, groupKey: item.groupKey });
      used += width;
      lastGroupKey = item.groupKey ?? null;
    } else {
      break;
    }
  }
  if (visible.length < items.length) {
    while (visible.length > 0 && used + moreWidth > available) {
      const removed = visible.pop();
      used -= removed.width;
    }
  }
  return visible.map((item) => item.key);
}
function fontMenuLabel(family) {
  return OFFICE_FONT_LABELS[family] ?? family;
}
function flattenOfficeItems(groups) {
  const items = [];
  for (const group of groups) {
    for (const item of group.items ?? [{ key: group.key, label: group.label ?? group.key, node: group.node }]) {
      items.push({ ...item, key: \`\${group.key}:\${item.key}\`, groupKey: group.key, groupLabel: group.label ?? group.key });
    }
  }
  return items;
}
function OfficeIconButton({ title, active, disabled, onClick, children }) {
  return /* @__PURE__ */ jsx2(Tooltip, { content: title, relationship: "label", children: /* @__PURE__ */ jsx2(Button, {
    appearance: active ? "primary" : "subtle",
    size: "small",
    icon: children,
    disabled,
    onClick,
    className: "phgo-office-icon-command"
  }) });
}
function OfficeCommand({ command, iconOnly = false }) {
  const button = /* @__PURE__ */ jsx2(Button, {
    appearance: command.active ? "primary" : "subtle",
    size: "small",
    icon: command.icon,
    disabled: command.disabled,
    onClick: command.onClick,
    className: \`phgo-office-command \${command.className ?? ""}\`,
    "aria-pressed": command.active ? true : void 0,
    children: iconOnly ? null : command.label
  });
  return command.title ? /* @__PURE__ */ jsx2(Tooltip, { content: command.title, relationship: "label", children: button }) : button;
}
function OfficeControl({ label, icon, children, className = "" }) {
  return /* @__PURE__ */ jsxs2("div", { className: \`phgo-office-control \${className}\`, children: [
    /* @__PURE__ */ jsxs2("div", { className: "phgo-office-control-main", children: [
      icon && /* @__PURE__ */ jsx2("span", { className: "phgo-office-control-icon", children: icon }),
      children
    ] }),
    /* @__PURE__ */ jsx2("div", { className: "phgo-office-control-label", children: label })
  ] });
}
function OfficeGroup({ label, children, className = "", hideLabel = false }) {
  return /* @__PURE__ */ jsxs2("section", { className: \`phgo-office-group \${hideLabel ? "phgo-office-group-no-title" : ""} \${className}\`, "aria-label": label, children: [
    /* @__PURE__ */ jsx2("div", { className: "phgo-office-group-controls", children }),
    !hideLabel && /* @__PURE__ */ jsx2("div", { className: "phgo-office-group-label", children: label })
  ] });
}
function OfficeButtonGroup({ children }) {
  return /* @__PURE__ */ jsx2("div", { className: "phgo-office-button-group", children });
}
function OfficeToggleGroup({ children, compact = false }) {
  return /* @__PURE__ */ jsx2("div", { className: \`phgo-office-toggle-group \${compact ? "phgo-office-toggle-group-compact" : ""}\`, children });
}
function OfficeNumber({ label, icon, value, min, max, step, decimals, onChange }) {
  const inputProps = { type: "number", size: "small", min, step, value: Number(value).toFixed(decimals), onChange: (_, data) => onChange(clampRibbonNumber(data.value, min, max)) };
  if (Number.isFinite(max)) inputProps.max = max;
  return /* @__PURE__ */ jsx2(OfficeControl, { label, icon, children:
    /* @__PURE__ */ jsx2(Input, inputProps)
  });
}
function OfficeZoom({ value, onChange }) {
  const percent = Math.round(Math.max(0.25, Math.min(4, value)) * 100);
  const pct = (percent - 25) / 375 * 100;
  const setPercent = (nextPercent) => onChange(clampRibbonNumber(nextPercent, 25, 400) / 100);
  const handleWheel = (event) => {
    event.preventDefault();
    const delta = event.deltaY < 0 ? 5 : -5;
    setPercent(percent + delta);
  };
  return /* @__PURE__ */ jsx2(OfficeControl, { label: "Zoom", icon: /* @__PURE__ */ jsx2(ZoomInRegular, {}), className: "phgo-office-control-zoom", children: /* @__PURE__ */ jsxs2(Fragment, { children: [
    /* @__PURE__ */ jsx2("input", { className: "phgo-office-zoom-slider", type: "range", min: 25, max: 400, step: 5, value: percent, style: { "--phgo-office-zoom-pct": \`\${pct}%\` }, onChange: (event) => setPercent(Number(event.currentTarget.value)), onWheel: handleWheel, onDoubleClick: () => onChange(1), "aria-label": "Zoom" }),
    /* @__PURE__ */ jsxs2("span", { className: "phgo-office-zoom-value", children: [percent, "%"] })
  ] }) });
}
function OfficeFontSize({ value, onChange }) {
  return /* @__PURE__ */ jsx2(OfficeControl, { label: "Font size", icon: /* @__PURE__ */ jsx2(TextFontSizeRegular, {}), className: "phgo-office-font-size-control", children:
    /* @__PURE__ */ jsx2(Dropdown, { className: "phgo-office-font-size", size: "small", style: { width: 54, minWidth: 54, maxWidth: 54 }, value: Number(value).toFixed(1), selectedOptions: [Number(value).toFixed(1)], onOptionSelect: (_, data) => onChange(clampRibbonNumber(data.optionValue, 0.5, 2.5)), children: [0.5, 0.8, 1, 1.2, 1.5, 2, 2.5].map((size) => /* @__PURE__ */ jsx2(Option, { value: size.toFixed(1), children: size.toFixed(1) }, size)) })
  });
}
function OfficeFontGrow({ value, onChange }) {
  return /* @__PURE__ */ jsxs2("div", { className: "phgo-office-font-grow", children: [
    /* @__PURE__ */ jsx2(Tooltip, { content: "Increase font size", relationship: "label", children: /* @__PURE__ */ jsx2(Button, { appearance: "subtle", size: "small", icon: /* @__PURE__ */ jsxs2("span", { className: "phgo-office-font-grow-icon", children: ["A", /* @__PURE__ */ jsx2(ArrowUpRegular, {})] }), onClick: () => onChange(Math.min(2.5, Math.round((value + 0.1) * 10) / 10)) }) }),
    /* @__PURE__ */ jsx2(Tooltip, { content: "Decrease font size", relationship: "label", children: /* @__PURE__ */ jsx2(Button, { appearance: "subtle", size: "small", icon: /* @__PURE__ */ jsxs2("span", { className: "phgo-office-font-shrink-icon", children: ["A", /* @__PURE__ */ jsx2(ChevronDownRegular, {})] }), onClick: () => onChange(Math.max(0.5, Math.round((value - 0.1) * 10) / 10)) }) })
  ] });
}
function OfficeExportSize({ value, onChange }) {
  const sizes = [1024, 2048, 4096, 8192];
  const selected = String(value || 4096);
  return /* @__PURE__ */ jsx2(OfficeControl, { label: "Size", icon: /* @__PURE__ */ jsx2(SlideSizeRegular, {}), className: "phgo-office-export-size-control", children:
    /* @__PURE__ */ jsx2(Dropdown, { className: "phgo-office-export-size", size: "small", style: { width: 64, minWidth: 64, maxWidth: 64 }, value: selected, selectedOptions: [selected], onOptionSelect: (_, data) => onChange(clampRibbonNumber(data.optionValue, 256, 16384)), children: sizes.map((size) => /* @__PURE__ */ jsx2(Option, { value: String(size), children: String(size) }, size)) })
  });
}
function OfficeExportMenu({ treeData, handleDownload, onViewerSnapshot, snapshotStateRef }) {
  const save = (format) => handleDownload(format);
  return /* @__PURE__ */ jsxs2(Menu, { positioning: "below-start", children: [
    /* @__PURE__ */ jsx2(MenuTrigger, { disableButtonEnhancement: true, children: /* @__PURE__ */ jsx2(MenuButton, { size: "small", appearance: "subtle", icon: /* @__PURE__ */ jsx2(ArrowDownloadRegular, {}), className: "phgo-office-command", children: "Export" }) }),
    /* @__PURE__ */ jsx2(MenuPopover, { children: /* @__PURE__ */ jsxs2(MenuList, { className: "phgo-office-export-menu", children: [
      /* @__PURE__ */ jsx2(MenuItem, { icon: /* @__PURE__ */ jsx2(ArrowDownloadRegular, {}), secondaryContent: "Vector", onClick: () => save("svg"), children: "SVG" }),
      /* @__PURE__ */ jsx2(MenuItem, { icon: /* @__PURE__ */ jsx2(ArrowDownloadRegular, {}), secondaryContent: "Raster", onClick: () => save("png"), children: "PNG" }),
      /* @__PURE__ */ jsx2(MenuItem, { icon: /* @__PURE__ */ jsx2(ArrowDownloadRegular, {}), secondaryContent: "Document", onClick: () => save("pdf"), children: "PDF" }),
      /* @__PURE__ */ jsx2(MenuDivider, {}),
      onViewerSnapshot && /* @__PURE__ */ jsx2(MenuItem, { icon: /* @__PURE__ */ jsx2(DocumentSaveRegular, {}), secondaryContent: "Full state", onClick: () => onViewerSnapshot?.(snapshotStateRef.current?.()), children: "PHgo Viewer Snapshot" }),
      /* @__PURE__ */ jsx2(MenuDivider, {}),
      /* @__PURE__ */ jsx2(MenuItem, { icon: /* @__PURE__ */ jsx2(DocumentSaveRegular, {}), secondaryContent: "Tree", onClick: () => {
        const nwk = toNewick(treeData) + ";";
        triggerDownload(URL.createObjectURL(new Blob([nwk], { type: "text/plain" })), "phylotree.nwk");
      }, children: "Newick" })
    ] }) })
  ] });
}
function OfficeOverflow({ label = "More", items, icon }) {
  if (!items.length) return null;
  return /* @__PURE__ */ jsxs2(Menu, { children: [
    /* @__PURE__ */ jsx2(MenuTrigger, { disableButtonEnhancement: true, children: /* @__PURE__ */ jsx2(MenuButton, { size: "small", appearance: "subtle", icon: icon ?? /* @__PURE__ */ jsx2(MoreHorizontalRegular, {}), children: label }) }),
    /* @__PURE__ */ jsx2(MenuPopover, { children: /* @__PURE__ */ jsx2("div", { className: "phgo-office-overflow-panel", children: items.map((item) => /* @__PURE__ */ jsxs2("div", { className: \`phgo-office-overflow-item \${item.divider ? "phgo-office-overflow-item-divider" : ""}\`, children: [
      /* @__PURE__ */ jsx2("span", { className: "phgo-office-overflow-item-label", children: item.label }),
      /* @__PURE__ */ jsx2("div", { className: "phgo-office-overflow-item-control", children: item.node })
    ] }, item.key)) }) })
  ] });
}
function OfficeTabOverflow({ tabs, onSelectTab }) {
  if (!tabs.length) return null;
  return /* @__PURE__ */ jsxs2(Menu, { children: [
    /* @__PURE__ */ jsx2(MenuTrigger, { disableButtonEnhancement: true, children: /* @__PURE__ */ jsx2(MenuButton, { size: "small", appearance: "subtle", icon: /* @__PURE__ */ jsx2(MoreHorizontalRegular, {}) }) }),
    /* @__PURE__ */ jsx2(MenuPopover, { children: /* @__PURE__ */ jsx2(MenuList, { children: tabs.map((tab) => /* @__PURE__ */ jsx2(MenuItem, { onClick: () => onSelectTab(tab.key), children: tab.label }, tab.key)) }) })
  ] });
}
function OfficeMeasureLayer({ tabs, items, measureRef }) {
  return /* @__PURE__ */ jsxs2("div", { className: "phgo-office-measure-layer", ref: measureRef, "aria-hidden": true, children: [
    /* @__PURE__ */ jsx2("div", { className: "phgo-office-measure-tabs", children: tabs.map((tab) => /* @__PURE__ */ jsx2("button", { "data-office-tab-measure": tab.key, className: "phgo-office-measure-tab", children: tab.label }, tab.key)) }),
    /* @__PURE__ */ jsx2("div", { className: "phgo-office-measure-groups", children: items.map((item) => /* @__PURE__ */ jsx2("div", { "data-office-item-measure": item.key, className: "phgo-office-measure-group", children: item.node }, item.key)) })
  ] });
}
function OfficeRibbon({
  activeRibbonTab,
  setActiveRibbonTab,
  ribbonRef,
  ribbonWidth,
  layout,
  setLayout,
  transformRef,
  hScale,
  setHScale,
  vScale,
  setVScale,
  fontScale,
  setFontScale,
  strokeWidth,
  setStrokeWidth,
  viewportScale,
  handleViewportScaleChange,
  fontFamily,
  setFontFamily,
  fontOptions,
  renderStyle,
  setRenderStyle,
  treeType,
  setTreeType,
  setAlignLabels,
  hasBranchLengths,
  hasBootstraps,
  labelMode,
  setLabelMode,
  fasta,
  showAlignment,
  setShowAlignment,
  alignLabels,
  showPhgoCoords,
  setShowPhgoCoords,
  collapsedNodes,
  setCollapsedNodes,
  history,
  dispatch,
  treeData,
  resetFnRef,
  rerootMode,
  setRerootMode,
  flipMode,
  setFlipMode,
  swapMode,
  setSwapMode,
  setSwapFirst,
  colorMode,
  setColorMode,
  activeIsReset,
  activeColor,
  setActiveColor,
  activeEntry,
  colorOverrides,
  setColorOverrides,
  colorBtnRef,
  handleColorModeToggle,
  downloadBtnRef,
  downloadOpen,
  setDownloadOpen,
  searchOpen,
  setSearchOpen,
  searchInputRef,
  searchQuery,
  setSearchQuery,
  searchMatchIdx,
  setSearchMatchIdx,
  searchMatchCount,
  handleDownload,
  exportLongEdge,
  setExportLongEdge,
  onViewerSnapshot,
  snapshotStateRef,
  rebuildWithLadderize,
  buildMidpointRerooted
}) {
  const tabRowRef = useRef(null);
  const toolbarRef = useRef(null);
  const measureRef = useRef(null);
  const expandFromCollapsedTabClickRef = useRef(false);
  const [officeLayout, setOfficeLayout] = useState({ visibleTabKeys: OFFICE_TABS.map((tab) => tab.key), visibleGroupKeys: [] });
  const [ribbonCollapsed, setRibbonCollapsed] = useState(false);
  const labelCycle = ["none", ...hasBootstraps ? ["bootstrap"] : [], ...hasBranchLengths ? ["branchlength"] : []];
  const nextLabelMode = labelCycle[(labelCycle.indexOf(labelMode) + 1) % labelCycle.length] ?? "none";
  const labelCaption = labelMode === "bootstrap" ? "Bootstrap" : labelMode === "branchlength" ? "Length" : "No labels";
  const updateSearch = (value) => {
    setSearchOpen(true);
    setSearchQuery(value);
    setSearchMatchIdx(0);
    setTimeout(() => searchInputRef.current?.focus(), 20);
  };
  const handleRibbonTabSelect = (_, data) => {
    setActiveRibbonTab(data.value);
    if (ribbonCollapsed) {
      expandFromCollapsedTabClickRef.current = true;
      setRibbonCollapsed(false);
      window.setTimeout(() => {
        expandFromCollapsedTabClickRef.current = false;
      }, 360);
    }
  };
  const handleRibbonTabClick = () => {
    if (!ribbonCollapsed) return;
    expandFromCollapsedTabClickRef.current = true;
    setRibbonCollapsed(false);
    window.setTimeout(() => {
      expandFromCollapsedTabClickRef.current = false;
    }, 360);
  };
  const handleRibbonTabDoubleClick = () => {
    if (expandFromCollapsedTabClickRef.current) {
      expandFromCollapsedTabClickRef.current = false;
      setRibbonCollapsed(false);
      return;
    }
    setRibbonCollapsed((collapsed) => !collapsed);
  };
  const handleRibbonOverflowTabSelect = (value) => {
    setActiveRibbonTab(value);
    if (ribbonCollapsed) setRibbonCollapsed(false);
  };
  const openItem = { key: "open", label: "Open", node: /* @__PURE__ */ jsx2(OfficeCommand, { command: { key: "open", label: "Open", icon: /* @__PURE__ */ jsx2(FolderOpenRegular, {}), disabled: true, title: "Tree file is provided by the current PHgo session" } }) };
  const snapshotItem = { key: "snapshot", label: "Snapshot", node: /* @__PURE__ */ jsx2(OfficeCommand, { command: { key: "snapshot", label: "Snapshot", icon: /* @__PURE__ */ jsx2(DocumentSaveRegular, {}), disabled: !onViewerSnapshot, onClick: () => onViewerSnapshot?.(snapshotStateRef.current?.()), title: "PHgo Viewer Snapshot" } }) };
  const exportItem = { key: "export", label: "Export", node: /* @__PURE__ */ jsx2(OfficeExportMenu, { treeData, handleDownload, onViewerSnapshot, snapshotStateRef }) };
  const exportSizeItem = { key: "exportSize", label: "Size", node: /* @__PURE__ */ jsx2(OfficeExportSize, { value: exportLongEdge, onChange: setExportLongEdge }) };
  const reloadItem = { key: "reload", label: "Reload", node: /* @__PURE__ */ jsx2(OfficeCommand, { command: { key: "reload", label: "Reload", icon: /* @__PURE__ */ jsx2(ArrowClockwiseRegular, {}), onClick: () => window.location.reload() } }) };
  const fileGroups = [
    { key: "file", label: "File", wrapClass: "phgo-office-button-group", node: /* @__PURE__ */ jsx2(OfficeGroup, { label: "File", children: /* @__PURE__ */ jsxs2(OfficeButtonGroup, { children: [openItem.node, snapshotItem.node, exportItem.node, exportSizeItem.node, reloadItem.node] }) }), items: [openItem, snapshotItem, exportItem, exportSizeItem, reloadItem] }
  ];
  const undoItem = { key: "undo", label: "Undo", node: /* @__PURE__ */ jsx2(OfficeCommand, { command: { key: "undo", label: "Undo", icon: /* @__PURE__ */ jsx2(ArrowUndoRegular, {}), disabled: history.length === 0, onClick: () => dispatch({ type: "UNDO" }) } }) };
  const ladderAscItem = { key: "ladderAsc", label: "Ladderize ascending", node: /* @__PURE__ */ jsx2(OfficeIconButton, { title: "Ladderize ascending", onClick: () => dispatch({ type: "LADDERIZE", newData: rebuildWithLadderize(treeData, true) }), children: /* @__PURE__ */ jsx2(IconLadderize, { asc: true }) }) };
  const ladderDescItem = { key: "ladderDesc", label: "Ladderize descending", node: /* @__PURE__ */ jsx2(OfficeIconButton, { title: "Ladderize descending", onClick: () => dispatch({ type: "LADDERIZE", newData: rebuildWithLadderize(treeData, false) }), children: /* @__PURE__ */ jsx2(IconLadderize, { asc: false }) }) };
  const rerootItem = { key: "reroot", label: "Reroot", node: /* @__PURE__ */ jsx2(OfficeCommand, { command: { key: "reroot", label: "Reroot", icon: /* @__PURE__ */ jsx2(BranchForkRegular, {}), active: rerootMode, onClick: () => {
    setRerootMode((mode) => !mode);
    if (!rerootMode) {
      setFlipMode(false);
      setSwapMode(false);
      setSwapFirst(null);
      setColorMode(false);
    }
  } } }) };
  const midpointItem = { key: "midpoint", label: "Midpoint", node: /* @__PURE__ */ jsx2(OfficeCommand, { command: { key: "midpoint", label: "Midpoint", icon: /* @__PURE__ */ jsx2(BranchForkRegular, {}), disabled: !hasBranchLengths, onClick: () => {
    setRerootMode(false);
    setFlipMode(false);
    setSwapMode(false);
    dispatch({ type: "REROOT", newData: buildMidpointRerooted(treeData) });
  } } }) };
  const flipItem = { key: "flip", label: "Flip", node: /* @__PURE__ */ jsx2(OfficeCommand, { command: { key: "flip", label: "Flip", icon: /* @__PURE__ */ jsx2(IconFlip, {}), active: flipMode, onClick: () => {
    setFlipMode((mode) => !mode);
    if (!flipMode) {
      setRerootMode(false);
      setSwapMode(false);
      setSwapFirst(null);
      setColorMode(false);
    }
  } } }) };
  const swapItem = { key: "swap", label: "Swap", node: /* @__PURE__ */ jsx2(OfficeCommand, { command: { key: "swap", label: "Swap", icon: /* @__PURE__ */ jsx2(IconSwap, {}), active: swapMode, onClick: () => {
    setSwapMode((mode) => !mode);
    if (!swapMode) {
      setRerootMode(false);
      setFlipMode(false);
      setSwapFirst(null);
      setColorMode(false);
    }
  } } }) };
  const colorItem = { key: "color", label: "Color", node: /* @__PURE__ */ jsx2(OfficeCommand, { command: { key: "color", label: "Color", icon: /* @__PURE__ */ jsx2(IconPalette, {}), active: colorMode, onClick: handleColorModeToggle, className: "phgo-office-color-command" } }) };
  const editGroups = [
    { key: "history", label: "History", wrapClass: "phgo-office-button-group", node: null, items: [undoItem] },
    { key: "sort", label: "Sort", wrapClass: "phgo-office-button-group", node: null, items: [ladderAscItem, ladderDescItem] },
    { key: "root", label: "Root", wrapClass: "phgo-office-button-group", node: null, items: [rerootItem, midpointItem] },
    { key: "arrange", label: "Arrange", wrapClass: "phgo-office-button-group", node: null, items: [flipItem, swapItem] },
    { key: "color", label: "Color", wrapClass: "phgo-office-button-group", node: null, items: [colorItem] }
  ];
  const zoomItem = { key: "zoom", label: "Zoom", node: /* @__PURE__ */ jsx2(OfficeZoom, { value: viewportScale, onChange: handleViewportScaleChange }) };
  const fitItem = { key: "fit", label: "Fit", node: /* @__PURE__ */ jsx2(OfficeCommand, { command: { key: "fit", label: "Fit", icon: /* @__PURE__ */ jsx2(ZoomFitRegular, {}), onClick: () => resetFnRef.current() } }) };
  const widthItem = { key: "width", label: layout === "circular" ? "Size" : "Width", node: /* @__PURE__ */ jsx2(OfficeNumber, { label: layout === "circular" ? "Size" : "Width", icon: layout === "circular" ? /* @__PURE__ */ jsx2(SlideSizeRegular, {}) : /* @__PURE__ */ jsx2(AutoFitWidthRegular, {}), value: hScale, min: 0, step: 0.1, decimals: 1, onChange: setHScale }) };
  const heightItem = { key: "height", label: "Height", node: layout === "rectangular" ? /* @__PURE__ */ jsx2(OfficeNumber, { label: "Height", icon: /* @__PURE__ */ jsx2(AutoFitHeightRegular, {}), value: vScale, min: 0, step: 0.1, decimals: 1, onChange: setVScale }) : null };
  const alignmentItem = { key: "alignment", label: "Alignment", node: /* @__PURE__ */ jsx2(OfficeCommand, { command: { key: "alignment", label: "Alignment", icon: /* @__PURE__ */ jsx2(IconAlnTrack, {}), disabled: !fasta || layout !== "rectangular", active: showAlignment, onClick: () => setShowAlignment((mode) => !mode) } }) };
  const alignItem = { key: "alignLabels", label: "Align", node: /* @__PURE__ */ jsx2(OfficeCommand, { command: { key: "alignLabels", label: "Align", icon: /* @__PURE__ */ jsx2(IconAlignLabels, {}), disabled: treeType !== "phylogram", active: alignLabels, onClick: () => setAlignLabels((mode) => !mode) } }) };
  const coordIcon = /* @__PURE__ */ jsxs2("svg", { width: "16", height: "16", viewBox: "0 0 16 16", fill: "none", children: [
    /* @__PURE__ */ jsx2("path", { d: "M3 4.25h10M3 8h10M3 11.75h10", stroke: "currentColor", strokeWidth: "1.35", strokeLinecap: "round" }),
    /* @__PURE__ */ jsx2("path", { d: "M4.2 2.8 2.8 4.25 4.2 5.7M11.8 2.8l1.4 1.45-1.4 1.45M4.2 6.55 2.8 8l1.4 1.45M11.8 6.55 13.2 8l-1.4 1.45M4.2 10.3l-1.4 1.45 1.4 1.45M11.8 10.3l1.4 1.45-1.4 1.45", stroke: "currentColor", strokeWidth: "1.1", strokeLinecap: "round", strokeLinejoin: "round" })
  ] });
  const coordItem = { key: "phgoCoords", label: "Coord", node: /* @__PURE__ */ jsx2(OfficeCommand, { command: { key: "phgoCoords", label: "Coord", icon: coordIcon, active: showPhgoCoords, onClick: () => setShowPhgoCoords?.((mode) => !mode), title: "Show PHgo row coordinates" } }) };
  const labelItem = { key: "labels", label: labelCaption, node: /* @__PURE__ */ jsx2(OfficeCommand, { command: { key: "labels", label: labelCaption, icon: labelMode === "bootstrap" ? /* @__PURE__ */ jsx2(IconBootstrap, {}) : labelMode === "branchlength" ? /* @__PURE__ */ jsx2(IconBranchLen, {}) : /* @__PURE__ */ jsx2(IconLabelsOff, {}), disabled: !hasBootstraps && !hasBranchLengths, onClick: () => setLabelMode(nextLabelMode) } }) };
  const expandItem = { key: "expand", label: collapsedNodes.size > 0 ? \`Expand all \${collapsedNodes.size}\` : "Expand all", node: /* @__PURE__ */ jsx2(OfficeCommand, { command: { key: "expand", label: collapsedNodes.size > 0 ? \`Expand all \${collapsedNodes.size}\` : "Expand all", disabled: collapsedNodes.size === 0, onClick: () => setCollapsedNodes(/* @__PURE__ */ new Set()) } }) };
  const viewGroups = [
    { key: "zoom", label: "Zoom", hideLabel: true, wrapClass: "phgo-office-button-group", node: /* @__PURE__ */ jsx2(OfficeGroup, { label: "Zoom", hideLabel: true, children: /* @__PURE__ */ jsxs2(OfficeButtonGroup, { children: [/* @__PURE__ */ jsx2(Fragment, { children: fitItem.node }), /* @__PURE__ */ jsx2(Fragment, { children: zoomItem.node })] }) }), items: [fitItem, zoomItem] },
    { key: "size", label: "Size", hideLabel: true, wrapClass: "phgo-office-inline-fields", node: /* @__PURE__ */ jsx2(OfficeGroup, { label: "Size", hideLabel: true, children: /* @__PURE__ */ jsxs2("div", { className: "phgo-office-inline-fields", children: [/* @__PURE__ */ jsx2(Fragment, { children: widthItem.node }), heightItem.node && /* @__PURE__ */ jsx2(Fragment, { children: heightItem.node })] }) }), items: [widthItem, ...(heightItem.node ? [heightItem] : [])] },
    { key: "labels", label: "Labels", wrapClass: "phgo-office-button-group", node: /* @__PURE__ */ jsx2(OfficeGroup, { label: "Labels", children: /* @__PURE__ */ jsxs2(OfficeButtonGroup, { children: [alignmentItem.node, alignItem.node, coordItem.node, labelItem.node, expandItem.node] }) }), items: [alignmentItem, alignItem, coordItem, labelItem, expandItem] }
  ];
  const rectItem = { key: "rect", label: "Rectangular", node: /* @__PURE__ */ jsx2(OfficeCommand, { command: { key: "rect", label: "Rectangular", icon: /* @__PURE__ */ jsx2(IconRect, {}), active: layout === "rectangular", className: "phgo-office-toggle-command", onClick: () => {
      setLayout("rectangular");
      transformRef.current = null;
    } } }) };
  const circItem = { key: "circ", label: "Circular", node: /* @__PURE__ */ jsx2(OfficeCommand, { command: { key: "circ", label: "Circular", icon: /* @__PURE__ */ jsx2(IconCirc, {}), active: layout === "circular", className: "phgo-office-toggle-command", onClick: () => {
      setLayout("circular");
      transformRef.current = null;
    } } }) };
  const phgoItem = { key: "phgo", label: "P", node: /* @__PURE__ */ jsx2(OfficeCommand, { command: { key: "phgo", label: "P", active: renderStyle === "phgo", className: "phgo-office-toggle-command phgo-office-letter-toggle", onClick: () => setRenderStyle("phgo"), title: "PHgo style" }, iconOnly: false }) };
  const megaItem = { key: "mega", label: "M", node: /* @__PURE__ */ jsx2(OfficeCommand, { command: { key: "mega", label: "M", active: renderStyle === "mega", className: "phgo-office-toggle-command phgo-office-letter-toggle", onClick: () => setRenderStyle("mega"), title: "MEGA style" }, iconOnly: false }) };
  const phyloItem = { key: "phylo", label: "Phylogram", node: /* @__PURE__ */ jsx2(OfficeCommand, { command: { key: "phylo", label: "Phylogram", icon: /* @__PURE__ */ jsx2(IconPhylo, {}), active: treeType === "phylogram", className: "phgo-office-toggle-command", onClick: () => setTreeType("phylogram") } }) };
  const cladoItem = { key: "clado", label: "Cladogram", node: /* @__PURE__ */ jsx2(OfficeCommand, { command: { key: "clado", label: "Cladogram", icon: /* @__PURE__ */ jsx2(IconClado, {}), active: treeType === "cladogram", className: "phgo-office-toggle-command", onClick: () => {
      setTreeType("cladogram");
      setAlignLabels(false);
    } } }) };
  const layoutItem = { key: "layoutToggle", label: "Layout", node: /* @__PURE__ */ jsxs2(OfficeToggleGroup, { children: [rectItem.node, circItem.node] }) };
  const modeItem = { key: "modeToggle", label: "Mode", node: /* @__PURE__ */ jsxs2(OfficeToggleGroup, { compact: true, children: [phgoItem.node, megaItem.node] }) };
  const treeItem = { key: "treeToggle", label: "Tree", node: /* @__PURE__ */ jsxs2(OfficeToggleGroup, { children: [phyloItem.node, cladoItem.node] }) };
  const fontFamilyItem = { key: "fontFamily", label: "Font", node: /* @__PURE__ */ jsx2(OfficeControl, { label: "Font", icon: /* @__PURE__ */ jsx2(TextFontRegular, {}), className: "phgo-office-font-control", children: /* @__PURE__ */ jsx2(Dropdown, { className: "phgo-office-font", size: "small", value: fontMenuLabel(fontFamily), selectedOptions: [fontFamily], onOptionSelect: (_, data) => setFontFamily(data.optionValue), children: fontOptions.map((family) => /* @__PURE__ */ jsx2(Option, { value: family, children: fontMenuLabel(family) }, family)) }) }) };
  const fontSizeItem = { key: "fontSize", label: "Font size", node: /* @__PURE__ */ jsx2(OfficeFontSize, { value: fontScale, onChange: setFontScale }) };
  const strokeIcon = /* @__PURE__ */ jsxs2("svg", { width: "14", height: "14", viewBox: "0 0 14 14", fill: "none", children: [
    /* @__PURE__ */ jsx2("path", { d: "M1 4h12", stroke: "currentColor", strokeWidth: "1", strokeLinecap: "round" }),
    /* @__PURE__ */ jsx2("path", { d: "M1 7.5h12", stroke: "currentColor", strokeWidth: "2", strokeLinecap: "round" }),
    /* @__PURE__ */ jsx2("path", { d: "M1 11.5h12", stroke: "currentColor", strokeWidth: "3", strokeLinecap: "round" })
  ] });
  const strokeItem = { key: "stroke", label: "Branch width", node: /* @__PURE__ */ jsx2(OfficeNumber, { label: "Branch", icon: strokeIcon, value: strokeWidth, min: 0.5, max: 4, step: 0.1, decimals: 1, onChange: setStrokeWidth }) };
  const fontGrowItem = { key: "fontGrow", label: "Adjust font", node: /* @__PURE__ */ jsx2(OfficeFontGrow, { value: fontScale, onChange: setFontScale }) };
  const formatGroups = [
    { key: "layout", label: "Layout", wrapClass: "phgo-office-toggle-wrap", node: /* @__PURE__ */ jsx2(OfficeGroup, { label: "Layout", children: layoutItem.node }), items: [layoutItem] },
    { key: "mode", label: "Mode", wrapClass: "phgo-office-toggle-wrap", node: /* @__PURE__ */ jsx2(OfficeGroup, { label: "Mode", children: modeItem.node }), items: [modeItem] },
    { key: "tree", label: "Tree", wrapClass: "phgo-office-toggle-wrap", node: /* @__PURE__ */ jsx2(OfficeGroup, { label: "Tree", children: treeItem.node }), items: [treeItem] },
    { key: "font", label: "Font", hideLabel: true, wrapClass: "phgo-office-font-fields", node: /* @__PURE__ */ jsx2(OfficeGroup, { label: "Font", hideLabel: true, children: /* @__PURE__ */ jsxs2("div", { className: "phgo-office-font-fields", children: [
      fontFamilyItem.node,
      fontSizeItem.node,
      fontGrowItem.node,
      strokeItem.node
    ] }) }), items: [fontFamilyItem, fontSizeItem, fontGrowItem, strokeItem] }
  ];
  const allGroups = activeRibbonTab === "file" ? fileGroups : activeRibbonTab === "edit" ? editGroups : activeRibbonTab === "format" ? formatGroups : viewGroups;
  const allItems = flattenOfficeItems(allGroups);
  const visibleItemKeySet = new Set(officeLayout.visibleGroupKeys);
  const visibleGroups = allGroups.map((group) => {
    const items = (group.items ?? [{ key: group.key, node: group.node, label: group.label ?? group.key }]).map((item) => ({ ...item, key: \`\${group.key}:\${item.key}\` })).filter((item) => visibleItemKeySet.has(item.key));
    if (!items.length) return null;
    return { ...group, node: /* @__PURE__ */ jsx2(OfficeGroup, { label: group.label ?? group.key, hideLabel: group.hideLabel, className: group.className, children: /* @__PURE__ */ jsx2("div", { className: group.wrapClass ?? "phgo-office-button-group", children: items.map((item) => /* @__PURE__ */ jsx2(Fragment, { children: item.node }, item.key)) }) }) };
  }).filter(Boolean);
  const overflowGroups = allItems.filter((item) => !visibleItemKeySet.has(item.key)).map((item, index, items) => ({ ...item, divider: index > 0 && item.groupKey !== items[index - 1].groupKey }));
  const visibleTabKeySet = new Set(officeLayout.visibleTabKeys);
  const visibleTabs = OFFICE_TABS.filter((tab) => visibleTabKeySet.has(tab.key));
  const hiddenTabs = OFFICE_TABS.filter((tab) => !visibleTabKeySet.has(tab.key));
  useLayoutEffect(() => {
    const measure = () => {
      const tabRow = tabRowRef.current;
      const toolbar = toolbarRef.current;
      const measureLayer = measureRef.current;
      if (!tabRow || !toolbar || !measureLayer) return;
      const tabWidths = {};
      measureLayer.querySelectorAll("[data-office-tab-measure]").forEach((el) => {
        tabWidths[el.dataset.officeTabMeasure] = Math.ceil(el.getBoundingClientRect().width);
      });
      const itemWidths = {};
      measureLayer.querySelectorAll("[data-office-item-measure]").forEach((el) => {
        itemWidths[el.dataset.officeItemMeasure] = Math.ceil(el.getBoundingClientRect().width);
      });
      const rowWidth = Math.floor(tabRow.getBoundingClientRect().width);
      const visibleTabKeys = measuredVisibleKeys(OFFICE_TABS, tabWidths, Math.max(0, rowWidth - OFFICE_SEARCH_BUTTON_WIDTH - 18), OFFICE_TAB_MORE_WIDTH);
      const toolbarWidth = Math.max(0, Math.floor((toolbar.parentElement ?? toolbar).getBoundingClientRect().width) - 44);
      const visibleGroupKeys = measuredVisibleKeys(allItems, itemWidths, toolbarWidth, OFFICE_MORE_BUTTON_WIDTH);
      setOfficeLayout((prev) => {
        const sameTabs = prev.visibleTabKeys.join("|") === visibleTabKeys.join("|");
        const sameGroups = prev.visibleGroupKeys.join("|") === visibleGroupKeys.join("|");
        if (sameTabs && sameGroups) return prev;
        return { visibleTabKeys, visibleGroupKeys };
      });
    };
    measure();
    const frame = requestAnimationFrame(measure);
    return () => cancelAnimationFrame(frame);
  }, [ribbonWidth, activeRibbonTab, ribbonCollapsed, allItems.length, layout, fontFamily, fontScale, strokeWidth, hScale, vScale, viewportScale, labelCaption, collapsedNodes.size, showAlignment, alignLabels, showPhgoCoords, renderStyle, treeType, history.length, colorMode, rerootMode, flipMode, swapMode]);
  return /* @__PURE__ */ jsx2(FluentProvider, { theme: webLightTheme, children: /* @__PURE__ */ jsxs2("div", { className: \`phgo-office-ribbon \${ribbonCollapsed ? "phgo-office-ribbon-collapsed" : ""}\`, ref: ribbonRef, children: [
    /* @__PURE__ */ jsx2(OfficeMeasureLayer, { tabs: OFFICE_TABS, items: allItems, measureRef }),
    /* @__PURE__ */ jsxs2("div", { className: "phgo-office-tab-row", ref: tabRowRef, children: [
      /* @__PURE__ */ jsxs2("div", { className: "phgo-office-tabs", children: [
        /* @__PURE__ */ jsx2(TabList, { size: "small", selectedValue: activeRibbonTab, onTabSelect: handleRibbonTabSelect, children: visibleTabs.map((tab) => /* @__PURE__ */ jsx2(Tab, { value: tab.key, onClick: handleRibbonTabClick, onDoubleClick: handleRibbonTabDoubleClick, children: tab.label }, tab.key)) }),
        /* @__PURE__ */ jsx2(OfficeTabOverflow, { tabs: hiddenTabs, onSelectTab: handleRibbonOverflowTabSelect })
      ] }),
      /* @__PURE__ */ jsx2("div", { className: "phgo-office-search phgo-office-search-button", children: /* @__PURE__ */ jsx2(Tooltip, { content: "Search taxa", relationship: "label", children: /* @__PURE__ */ jsx2(Button, { appearance: "subtle", size: "small", icon: /* @__PURE__ */ jsx2(SearchRegular, {}), onClick: () => {
        setSearchOpen(true);
        setTimeout(() => searchInputRef.current?.focus(), 30);
      } }) }) })
    ] }),
    /* @__PURE__ */ jsxs2("div", { className: "phgo-office-toolbar-row", children: [
      /* @__PURE__ */ jsx2("div", { className: "phgo-office-strip", ref: toolbarRef, children: visibleGroups.map((group) => /* @__PURE__ */ jsx2(Fragment, { children: group.node }, group.key)) }),
      overflowGroups.length > 0 && /* @__PURE__ */ jsx2(OfficeOverflow, { items: overflowGroups })
    ] }),
    /* @__PURE__ */ jsx2("button", { ref: colorBtnRef, className: "phgo-office-anchor", style: { color: colorMode && !activeIsReset ? activeColor : void 0 }, tabIndex: -1, "aria-hidden": true }),
    /* @__PURE__ */ jsx2("button", { ref: downloadBtnRef, className: "phgo-office-anchor", tabIndex: -1, "aria-hidden": true })
  ] }) });
}`;
  const iconAnchor = '// src/Reactree.tsx\nimport { Fragment, jsx as jsx2, jsxs as jsxs2 } from "react/jsx-runtime";';
  const existingOfficeIndex = text.indexOf('const OFFICE_TABS = [');
  const iconAnchorIndex = text.indexOf(iconAnchor, existingOfficeIndex);
  if (existingOfficeIndex !== -1 && iconAnchorIndex !== -1) {
    text = `${text.slice(0, existingOfficeIndex)}${officeRibbonCode}\n${text.slice(iconAnchorIndex)}`;
  } else if (!text.includes(officeRibbonMarker)) {
    text = replaceRequired(
      text,
      iconAnchor,
      `${officeRibbonCode}\n${iconAnchor}`,
      'Reactree Office ribbon helpers',
      file,
    );
  }
  text = text
    .replace(
      '            false && searchOpen && /* @__PURE__ */ jsxs2("div", { className: Reactree_default.searchPanel, children: [',
      '            searchOpen && /* @__PURE__ */ jsxs2("div", { className: Reactree_default.searchPanel, children: [',
    )
    .replace(
      '    mounted && downloadOpen && downloadMenuPos && createPortal(',
      '    false && mounted && downloadOpen && downloadMenuPos && createPortal(',
    )
    .replace(
      '    false && mounted && colorMode && palettePos && createPortal(',
      '    mounted && colorMode && palettePos && createPortal(',
    );

  const ribbonStateAnchor = '  const [mounted, setMounted] = useState(false);';
  const ribbonStatePatch = `  const [mounted, setMounted] = useState(false);
  const [activeRibbonTab, setActiveRibbonTab] = useState("view");
  const ribbonRef = useRef(null);
  const [ribbonWidth, setRibbonWidth] = useState(0);`;
  if (!text.includes('const [activeRibbonTab, setActiveRibbonTab]')) {
    text = replaceRequired(text, ribbonStateAnchor, ribbonStatePatch, 'Reactree Office ribbon state', file);
  }
  const mountedEffectAnchor = `  useEffect(() => {
    setMounted(true);
  }, []);`;
  const ribbonResizeEffect = `  useEffect(() => {
    setMounted(true);
  }, []);
  useLayoutEffect(() => {
    const element = ribbonRef.current;
    if (!element) return void 0;
    const update = () => setRibbonWidth(element.getBoundingClientRect().width || 0);
    update();
    const observer = new ResizeObserver(update);
    observer.observe(element);
    return () => observer.disconnect();
  }, []);`;
  if (!text.includes('setRibbonWidth(element.getBoundingClientRect().width')) {
    text = replaceRequired(text, mountedEffectAnchor, ribbonResizeEffect, 'Reactree Office ribbon resize observer', file);
  }

  const returnStart = '  return /* @__PURE__ */ jsxs2("div", { className: Reactree_default.outer, children: [\n';
  const bodyStart = '    /* @__PURE__ */ jsxs2("div", { className: `${Reactree_default.bodyRow} ${showAlignment && parsedFasta && layout === "rectangular" ? Reactree_default.bodyRowWithAln : ""}`, children: [\n';
  const startIndex = text.indexOf(returnStart);
  const bodyIndex = text.indexOf(bodyStart, startIndex);
  if (startIndex === -1 || bodyIndex === -1) {
    if (!text.includes('/* @__PURE__ */ jsx2(OfficeRibbon')) {
      throw new Error(`Reactree Office ribbon return anchors not found in ${file}`);
    }
  } else {
    const replacementPrefix = `  return /* @__PURE__ */ jsxs2("div", { className: Reactree_default.outer, children: [
    /* @__PURE__ */ jsx2(OfficeRibbon, {
      activeRibbonTab,
      setActiveRibbonTab,
      ribbonRef,
      ribbonWidth,
      layout,
      setLayout,
      transformRef,
      hScale,
      setHScale,
      vScale,
      setVScale,
      fontScale,
      setFontScale,
      strokeWidth,
      setStrokeWidth,
      viewportScale,
      handleViewportScaleChange,
      fontFamily,
      setFontFamily,
      fontOptions,
      renderStyle,
      setRenderStyle,
      treeType,
      setTreeType,
      setAlignLabels,
      hasBranchLengths,
      hasBootstraps,
      labelMode,
      setLabelMode,
      fasta,
      showAlignment,
      setShowAlignment,
      alignLabels,
      showPhgoCoords,
      setShowPhgoCoords,
      collapsedNodes,
      setCollapsedNodes,
      history,
      dispatch,
      treeData,
      resetFnRef,
      rerootMode,
      setRerootMode,
      flipMode,
      setFlipMode,
      swapMode,
      setSwapMode,
      setSwapFirst,
      colorMode,
      setColorMode,
      activeIsReset,
      activeColor,
      setActiveColor,
      activeEntry,
      colorOverrides,
      setColorOverrides,
      colorBtnRef,
      handleColorModeToggle,
      downloadBtnRef,
      downloadOpen,
      setDownloadOpen,
      searchOpen,
      setSearchOpen,
      searchInputRef,
      searchQuery,
      setSearchQuery,
      searchMatchIdx,
      setSearchMatchIdx,
      searchMatchCount,
      handleDownload,
      exportLongEdge,
      setExportLongEdge,
      onViewerSnapshot,
      snapshotStateRef,
      rebuildWithLadderize,
      buildMidpointRerooted
    }),
`;
    text = `${text.slice(0, startIndex)}${replacementPrefix}${text.slice(bodyIndex)}`;
  }
  if (!text.includes('      exportLongEdge,\n      setExportLongEdge,\n      onViewerSnapshot,')) {
    text = replaceRequired(
      text,
      '      handleDownload,\n      onViewerSnapshot,\n',
      '      handleDownload,\n      exportLongEdge,\n      setExportLongEdge,\n      onViewerSnapshot,\n',
      'Reactree Office ribbon export size props',
      file,
    );
  }

  if (text !== originalText) {
    writeFileSync(file, text);
    console.log(`Patched Reactree Office ribbon: ${file}`);
  } else {
    console.log(`Reactree Office ribbon already present: ${file}`);
  }
}

patchReactreeOfficeRibbonMJS(join(packageRoot, 'dist', 'index.mjs'));

{
  const file = join(packageRoot, 'dist', 'index.mjs');
  const text = readFileSync(file, 'utf8').replace(/\r\n/g, '\n');
  const next = ensureSnapshotStateBridge(text, file);
  if (next !== text) {
    writeFileSync(file, next);
  }
}

function patchReactreePHGOLabels(file) {
  let text = readFileSync(file, 'utf8').replace(/\r\n/g, '\n');

  const displayNameOriginal = `function displayTreeName(name) {
  return String(name || "");
}`;

  const displayNamePatched = `let phgoDisplayTreeLabelResolver = null;
function setDisplayTreeLabelResolver(resolver) {
  phgoDisplayTreeLabelResolver = typeof resolver === "function" ? resolver : null;
}
function escapeDisplayLabelRegExp(value) {
  return String(value || "").replace(/[.*+?^$()|[\\]\\\\]/g, (match) => "\\\\" + match);
}
function buildPhgoDisplayTreeLabelResolver(metadata, showPhgoCoords) {
  const records = Array.isArray(metadata?.records) ? metadata.records : [];
  const labels = /* @__PURE__ */ new Map();
  for (const record of records) {
    const taxon = String(record?.taxon_id || "").trim();
    if (!taxon) continue;
    const prefix = String(record?.display_prefix || "").trim();
    const rawName = String(record?.display_name || taxon).trim() || taxon;
    const baseName = prefix ? rawName.replace(new RegExp(\`^\${escapeDisplayLabelRegExp(prefix)}(?:\\\\s+)?\`), "").trim() || taxon : rawName;
    labels.set(taxon, showPhgoCoords && prefix ? \`\${prefix} \${baseName}\` : baseName);
  }
  return (name) => {
    const raw = String(name || "").trim();
    return labels.get(raw) || raw;
  };
}
function displayTreeName(name) {
  const raw = String(name || "");
  return phgoDisplayTreeLabelResolver ? phgoDisplayTreeLabelResolver(raw) : raw;
}`;

  if (text.includes(displayNameOriginal)) {
    text = replaceRequired(text, displayNameOriginal, displayNamePatched, 'Reactree PHGO display-name resolver', file);
  } else if (!text.includes('function buildPhgoDisplayTreeLabelResolver(')) {
    throw new Error(`Reactree PHGO display-name resolver anchor not found in ${file}`);
  }

  text = replaceRequired(
    text,
    'function Reactree({ newick, defaultHeight = 520, fasta, initialState, onStateChange, onViewerSnapshot, showPhgoCoords = false, setShowPhgoCoords }) {',
    'function Reactree({ newick, defaultHeight = 520, fasta, initialState, onStateChange, onViewerSnapshot, labelMetadata, showPhgoCoords = false, setShowPhgoCoords }) {',
    'Reactree labelMetadata prop',
    file,
  );

  if (!text.includes('const displayLabelResolver = useMemo(() => buildPhgoDisplayTreeLabelResolver(labelMetadata, showPhgoCoords)')) {
    text = replaceRequired(
      text,
      '  const initialSnapshot = normalizeReactreeStateSnapshot(initialState);',
      `  const initialSnapshot = normalizeReactreeStateSnapshot(initialState);
  const displayLabelResolver = useMemo(() => buildPhgoDisplayTreeLabelResolver(labelMetadata, showPhgoCoords), [labelMetadata, showPhgoCoords]);
  useEffect(() => {
    setDisplayTreeLabelResolver(displayLabelResolver);
    return () => {
      setDisplayTreeLabelResolver(null);
    };
  }, [displayLabelResolver]);`,
      'Reactree display-label resolver state',
      file,
    );
  }

  text = replaceRequired(
    text,
    '  }, [getPreparedAlignmentRows]);',
    '  }, [getPreparedAlignmentRows, showPhgoCoords, labelMetadata]);',
    'Reactree alignment redraw deps for PHGO labels',
    file,
  );

  text = replaceRequired(
    text,
    '  }, [treeData, containerH, hScale, vScale, fontScale, fontFamily, strokeWidth, layout, renderStyle, treeType, colorOverrides, theme, labelMode, alignLabels, truncateNames, collapsedNodes, collapseLabels, searchQuery, searchMatchIdx, showAlignment]);',
    '  }, [treeData, containerH, hScale, vScale, fontScale, fontFamily, strokeWidth, layout, renderStyle, treeType, colorOverrides, theme, labelMode, alignLabels, truncateNames, collapsedNodes, collapseLabels, searchQuery, searchMatchIdx, showAlignment, showPhgoCoords, labelMetadata]);',
    'Reactree tree redraw deps for PHGO labels',
    file,
  );

  writeFileSync(file, text);
  console.log(`Patched Reactree PHGO labels: ${file}`);
}

patchReactreePHGOLabels(join(packageRoot, 'dist', 'index.mjs'));

for (const file of ['index.d.ts', 'index.d.mts'].map((name) => join(packageRoot, 'dist', name))) {
  let text = readFileSync(file, 'utf8');
  if (!text.includes('onViewerSnapshot')) {
    text = text
      .replace('    fasta?: string;\n', '    fasta?: string;\n    initialState?: Record<string, unknown>;\n    onStateChange?: (state: Record<string, unknown>) => void;\n    onViewerSnapshot?: (state: Record<string, unknown>) => void;\n    labelMetadata?: Record<string, unknown>;\n    showPhgoCoords?: boolean;\n    setShowPhgoCoords?: (updater: boolean | ((value: boolean) => boolean)) => void;\n')
      .replace('declare function Reactree({ newick, defaultHeight, fasta }: ReactreeProps)', 'declare function Reactree({ newick, defaultHeight, fasta, initialState, onStateChange, onViewerSnapshot, labelMetadata, showPhgoCoords, setShowPhgoCoords }: ReactreeProps)');
    writeFileSync(file, text);
    console.log(`Patched Reactree viewer-state types: ${file}`);
  } else if (!text.includes('labelMetadata?: Record<string, unknown>;') || !text.includes('showPhgoCoords?: boolean;')) {
    text = text
      .replace('    onViewerSnapshot?: (state: Record<string, unknown>) => void;\n', '    onViewerSnapshot?: (state: Record<string, unknown>) => void;\n    labelMetadata?: Record<string, unknown>;\n    showPhgoCoords?: boolean;\n    setShowPhgoCoords?: (updater: boolean | ((value: boolean) => boolean)) => void;\n')
      .replace('declare function Reactree({ newick, defaultHeight, fasta, initialState, onStateChange, onViewerSnapshot }: ReactreeProps)', 'declare function Reactree({ newick, defaultHeight, fasta, initialState, onStateChange, onViewerSnapshot, labelMetadata, showPhgoCoords, setShowPhgoCoords }: ReactreeProps)')
      .replace('declare function Reactree({ newick, defaultHeight, fasta, initialState, onStateChange, onViewerSnapshot, showPhgoCoords, setShowPhgoCoords }: ReactreeProps)', 'declare function Reactree({ newick, defaultHeight, fasta, initialState, onStateChange, onViewerSnapshot, labelMetadata, showPhgoCoords, setShowPhgoCoords }: ReactreeProps)');
    writeFileSync(file, text);
    console.log(`Patched Reactree coordinate viewer-state types: ${file}`);
  } else {
    console.log(`Reactree viewer-state types already present: ${file}`);
  }
}
