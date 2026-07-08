let snapshot = null;
let currentView = "spec";
let cy = null;
let selectedNodeID = null;
let activeInspectorTab = "info";
let activeTruthOwnerID = null;
let activeDocMode = "rendered";
let mermaidReady = false;
let activeSpecflowNavGroup = "unit";
let activeReviewNavGroup = "candidate";
let expandedReviewObjectKeys = new Set();
let initializedReviewNavGroups = new Set();
let activeRegistryNavGroup = "problem";
let snapshotRequestInFlight = false;
let snapshotDataSignature = "";
let activeSourceHeadings = [];
let docGuideOpen = false;
let lastGraphView = "";
let activeSourceDiff = null;
let diffMarkersEnabled = true;
let expandedDiffMarkers = new Set();

const LANGUAGE_STORAGE_KEY = "specflow-reader-language";
const SUPPORTED_LANGUAGES = ["zh-CN", "en"];
const SNAPSHOT_POLL_INTERVAL_MS = 5000;
let currentLanguage = readStoredLanguage();

const TRANSLATIONS = {
  "zh-CN": {
    loading: "读取中",
    viewNavAria: "Reader 视图",
    graphLegendAria: "图节点颜色说明",
    resizeAria: "调整检查面板宽度",
    inspectorTabsAria: "节点检查",
    docModeAria: "Spec 文档显示模式",
    docGuideAria: "Spec 文档导览",
    refresh: "刷新",
    language: {
      label: "语言",
      zh: "中文"
    },
    tabs: {
      spec: { title: "Spec 查看", subtitle: "文档入口" },
      registry: { title: "结构映射", subtitle: "实施路径" },
      project: { title: "项目结构", subtitle: "仓库路径" },
      specflow: { subtitle: "治理层级" }
    },
    legend: {
      unit: {
        label: "单元",
        tooltip: "单元是一块可独立说明、开发和验证的工程责任，例如 agent、memory 或 tool。"
      },
      rule: {
        label: "规则",
        tooltip: "规则是多个单元共同复用的一段约束，避免同一规则在不同地方重复写。"
      },
      shared: {
        label: "规则",
        tooltip: "规则是多个单元共同复用的一段约束，避免同一规则在不同地方重复写。"
      },
      truth: {
        label: "Spec 文档",
        tooltip: "Spec 文档是当前项目认可的说明文档；开发和验证都要以这些文档为准。"
      },
      path: {
        label: "实现路径",
        tooltip: "实现路径是代码、配置或工具文件所在的位置，表示规则落到哪些工程文件里。"
      },
      system: {
        label: "全局规则",
        tooltip: "全局规则是全仓库通用的技术底线，例如默认选择、禁止事项和全局例外。"
      }
    },
    views: {
      spec: {
        title: "Spec 查看",
        summary: "查看当前需要确认的 candidate Spec，以及已经成为 stable 的正式 Spec。",
        nav: "Spec 文档"
      },
      project: {
        title: "项目结构",
        summary: "从仓库路径看实现位置：哪些代码或工程路径已经归到具体责任对象，先不展示 SpecFlow 自己的 Spec 文档和支撑文件。",
        nav: "目录",
        groups: {
          areas: "实现区域"
        }
      },
      specflow: {
        title: "SpecFlow",
        summary: "从治理层级看规则：全局规则、项目映射、状态索引、规则、单元和 Spec 文档如何分层。",
        nav: "对象"
      },
      registry: {
        title: "结构映射",
        summary: "查看 unit 和 rule 是否已经写入 repository_mapping，以及 unit 有没有可用实施路径。",
        nav: "映射结果"
      }
    },

    counts: {
      unit: "{count} 单元",
      rule: "{count} 规则",
      truth: "{count} Spec 文档",
      paths: "{count} 个路径或文件",
      objects: "{count} 个对象"
    },
    specflowSections: {
      unit: "单元",
      rule: "规则",
      truth: "Spec 文档",
      implementation: "实现路径",
      system: "全局规则",
      support: "支撑文件"
    },
    fallback: {
      statusUnknown: "状态未声明",

      none: "无",
      responsibilityUnknown: "职责未声明",
      undeclared: "未声明",
      rule: "规则",
      noObject: "暂无对象",
      cytoscapeMissing: "Cytoscape 未加载。"
    },

    registry: {
      boardHeading: "结构映射面板",
      boardDescription: "查看每个对象是否已经写入 repository_mapping，以及是否已经声明实施路径，避免执行时才发现映射缺口。",
      knownUnits: "已知单元",
      missingMapping: "未写入 repository_mapping",
      mappedWithoutPath: "已映射但无实施路径",
      mappedWithPath: "已映射且有实施路径",
      missingMappingHeading: "未写入 repository_mapping",
      missingMappingDescription: "这些对象已经出现在状态或 Spec 文件里，但还没有进入项目结构映射，执行前需要先补清归属。",
      mappedNoPathHeading: "已映射，但没有可用实施路径",
      mappedNoPathDescription: "这些对象已经进入项目结构映射，但还没有声明实施路径，或者声明的路径在当前仓库里不存在。",
      mappedWithPathHeading: "已映射，且已有实施路径",
      mappedWithPathDescription: "这些对象已经进入项目结构映射，并且有可用实施路径。",
      result: "映射状态",
      mapping: "repository_mapping",
      status: "状态登记",
      truth: "Spec 文档",
      implementation: "实施路径",
      objectLabel: "对象",
      refs: "引用",
      evidence: "发现依据",
      relation: "当前关系",
      attention: "需要关注",
      issues: "缺口",
      complete: "无断链",
      gap: "有断链",
      planned: "已映射，无实施路径",
      landed: "已映射，有实施路径",
      missingFile: "实施路径不存在",
      unregisteredFile: "未写入映射",
      invalidRegistryRow: "映射表错误",
      all: "全部",
      yes: "有",
      no: "缺失",
      optional: "未登记",
      declared: "已登记",
      notApplicable: "不适用",
      noIssues: "暂无缺口",
      sourceChain: "映射链",
      mappingSource: "项目结构来源",
      statusSource: "状态来源",
      truthSources: "Spec 文件",
      unitRefs: "单元引用",
      ruleRefs: "规则引用",
      boundObjects: "绑定对象",
      globalActive: "全局生效",
      unboundRule: "未绑定",
      noMissingMapping: "没有未映射对象。",
      noMappedNoPath: "没有实施路径缺口。",
      noMappedWithPath: "没有已落实施路径对象。",
      noPlanned: "没有已映射但无实施路径的对象。",
      noLanded: "没有已映射且有实施路径的对象。",
      noProblems: "没有映射问题。",
      unmappedAttention: "未进入 repository_mapping",
      mappedAttention: "已纳入 repository_mapping",
      ruleScope: {
        global: "全局规则",
        bound: "绑定规则",
        unknown: "规则"
      },
      filters: {
        problem: "映射缺口",
        planned: "已映射无路径",
        landed: "已映射有路径",
        unit: "单元",
        rule: "规则"
      }
    },
    review: {
      empty: "暂无可查看 Spec。",
      emptyNav: "暂无 Spec 文档。",
      emptyDetailTitle: "暂无 Spec 文档",
      emptyDetail: "当前快照里没有可查看的 candidate 或 stable 主 Spec。",
      openSource: "打开 Spec 原文",
      fileType: "文档分组",
      object: "对应项目对象",
      reviewTarget: "查看说明",
      readingFocus: "查看重点",
      relationships: "相关关系",
      relationEmpty: "暂无相关关系快照。",

      relation: {
        implementation: "实现路径",
        rule: "规则",
        ruleFile: "规则文件",
        bound: "绑定对象",
        appendix: "附录文件",
        evidence: "证据参考",
        stable: "稳定基线参考",
        mapping: "项目结构参考",
        system: "全局规则参考"
      },
      types: {
        candidate: "待确认 candidate",
        stable: "已确认 stable",
        stableRule: "已确认规则",
        capability: "单元设计",
        rule: "规则",
        structure: "项目结构文件",
        system: "全局规则文件"
      },
      states: {
        candidate: "待确认",
        stable: "已确认",
        stableRule: "已确认"
      },
      docKinds: {
        main: "主文",
        appendix: "附录",
        evidence: "证据",
        evidenceAppendix: "证据附录"
      },
      targets: {
        candidate: "这是当前正在确认的 Spec，确认完成前不能当作正式基线。",
        stable: "这是已经确认的正式 Spec，可作为当前正式基线查看。",
        stableRule: "这是已经确认的共享规则 Spec，可作为当前正式规则查看。",
        capability: "整份文件是否正确表达该能力的当前设计或规则。",
        rule: "整份文件是否正确表达这条规则及其复用边界。",
        structure: "整份文件是否正确表达当前项目结构、对象边界和路径归属。",
        system: "整份文件是否正确表达全仓库规则、默认选择和例外。"
      },
      focus: {
        candidate: "当前设计、边界、验收条件、附录、规则引用",
        stable: "正式设计、下一步动作、附录、证据、规则引用",
        stableRule: "规则正文、复用边界、绑定对象",
        capability: "责任边界、输入输出、错误处理、验收条件、规则引用",
        rule: "复用对象、规则正文、绑定关系、是否仍是局部规则",
        structure: "单元列表、规则列表、路径归属、支撑文件边界",
        system: "技术基线、默认选择、复用机制、禁止项、例外"
      }
    },

    inspector: {
      infoTab: "节点信息",
      truthTab: "Spec 文档",
      truthTitle: "Spec 文档",
       fields: {
        type: "类型",
        status: "状态",
        version: "版本",
        responsibility: "职责",
        file: "文件",
        connections: "连接",
        paths: "路径"
      },
      groups: {
        truth: "Spec 文档",
        implementation: "实现路径",
        rule: "规则",
        bound: "绑定对象",
        connected: "关联节点"
      }
    },
    docMode: {
      rendered: "渲染",
      raw: "原文"
    },
    diff: {
      show: "显示差异",
      hide: "隐藏差异",
      unavailable: "无 stable 可对比",
      added: "新增",
      deleted: "删除",
      modified: "修改",
      context: "上下文",
      summary: "相对 stable 的变化",
      stableRange: "stable",
      candidateRange: "candidate",
      insertedLines: "新增 {count} 行",
      deletedLines: "删除 {count} 行",
      expand: "查看完整差异"
    },
    source: {
      guideTitle: "导览",
      guideShow: "显示导览",
      guideHide: "隐藏导览",
      guideUnavailable: "无导览",
      noGuide: "暂无标题",
      emptyRendered: "选择一个 Spec 文档查看内容。",
      emptyRaw: "选择一个 Spec 文档查看原文。"
    },
    kind: {
      project_root: "仓库目录",
      project_path: "路径",
      project_area: "实现区域",
      repository_mapping: "项目结构文件",
      status_index: "状态索引",
      rule: "全局规则",
      truth_file: "Spec 文档"
    },
    frontmatter: {
      title: "元信息",
      undeclared: "未声明"
    }
  },
  en: {
    loading: "Loading",
    viewNavAria: "Reader views",
    graphLegendAria: "Graph node color legend",
    resizeAria: "Resize inspector panel",
    inspectorTabsAria: "Node inspector",
    docModeAria: "Spec document display mode",
    docGuideAria: "Spec document guide",
    refresh: "Refresh",
    language: {
      label: "Language",
      zh: "Chinese"
    },
    tabs: {
      spec: { title: "Spec View", subtitle: "Documents" },
      registry: { title: "Mapping", subtitle: "Implementation paths" },
      project: { title: "Project", subtitle: "Repository paths" },
      specflow: { subtitle: "Governance layers" }
    },
    legend: {
      unit: {
        label: "Unit",
        tooltip: "A unit is an engineering responsibility that can be described, developed, and verified independently, such as agent, memory, or tool."
      },
      rule: {
        label: "Rule",
        tooltip: "A rule is reused by multiple units so the same constraint is not duplicated in different places."
      },
      shared: {
        label: "Rule",
        tooltip: "A rule is reused by multiple units so the same constraint is not duplicated in different places."
      },
      truth: {
        label: "Spec document",
        tooltip: "A Spec document is accepted project text. Development and verification must follow these documents."
      },
      path: {
        label: "Implementation path",
        tooltip: "An implementation path is where code, configuration, or tooling files live."
      },
      system: {
        label: "Global rules",
        tooltip: "Global rules are repository-wide technical baselines, such as defaults, prohibitions, and global exceptions."
      }
    },
    views: {
      spec: {
        title: "Spec View",
        summary: "Shows candidate Specs that still need confirmation and stable Specs that are already accepted.",
        nav: "Spec documents"
      },
      project: {
        title: "Project Structure",
        summary: "Shows implementation locations from repository paths: which code or engineering paths are assigned to responsibility objects. SpecFlow's own Spec documents and support files are not shown here.",
        nav: "Directories",
        groups: {
          areas: "Implementation areas"
        }
      },
      specflow: {
        title: "SpecFlow",
        summary: "Shows governance layers: how global rules, repository mapping, status index, rules, units, and Spec documents are organized.",
        nav: "Objects"
      },
      registry: {
        title: "Structure Mapping",
        summary: "Shows whether units and rules are recorded in repository_mapping and whether units have usable implementation paths.",
        nav: "Mapping results"
      }
    },

    counts: {
      unit: "{count} units",
      rule: "{count} rules",
      truth: "{count} Spec documents",
      paths: "{count} paths or files",
      objects: "{count} objects"
    },
    specflowSections: {
      unit: "Units",
      rule: "Rules",
      truth: "Spec documents",
      implementation: "Implementation paths",
      system: "Global rules",
      support: "Support files"
    },
    fallback: {
      statusUnknown: "Status not declared",

      none: "None",
      responsibilityUnknown: "Responsibility not declared",
      undeclared: "Not declared",
      rule: "Rule",
      noObject: "No object",
      cytoscapeMissing: "Cytoscape is not loaded."
    },

    registry: {
      boardHeading: "Structure Mapping Panel",
      boardDescription: "Shows whether each object is recorded in repository_mapping and whether it already declares implementation paths, so mapping gaps are visible before execution.",
      knownUnits: "Known units",
      missingMapping: "Not in repository_mapping",
      mappedWithoutPath: "Mapped without paths",
      mappedWithPath: "Mapped with paths",
      missingMappingHeading: "Not in repository_mapping",
      missingMappingDescription: "These objects appear in status or Spec files, but are not recorded in repository_mapping yet. Their ownership should be clarified before execution.",
      mappedNoPathHeading: "Mapped, but no usable implementation path",
      mappedNoPathDescription: "These objects are recorded in repository_mapping, but either have no implementation path or point to a path that does not exist in this repository.",
      mappedWithPathHeading: "Mapped with implementation paths",
      mappedWithPathDescription: "These objects are recorded in repository_mapping and have usable implementation paths.",
      result: "Mapping state",
      mapping: "repository_mapping",
      status: "Status registration",
      truth: "Spec files",
      implementation: "Implementation paths",
      objectLabel: "Object",
      refs: "References",
      evidence: "Evidence",
      relation: "Current relation",
      attention: "Needs attention",
      issues: "Gaps",
      complete: "No broken links",
      gap: "Broken links",
      planned: "Mapped, no paths",
      landed: "Mapped with paths",
      missingFile: "Path does not exist",
      unregisteredFile: "Not mapped",
      invalidRegistryRow: "Mapping row error",
      all: "All",
      yes: "Yes",
      no: "Missing",
      optional: "Not registered",
      declared: "Registered",
      notApplicable: "N/A",
      noIssues: "No gaps",
      sourceChain: "Mapping chain",
      mappingSource: "Structure source",
      statusSource: "Status source",
      truthSources: "Spec files",
      unitRefs: "Unit refs",
      ruleRefs: "Rule refs",
      boundObjects: "Bound objects",
      globalActive: "Global active",
      unboundRule: "Unbound",
      noMissingMapping: "No unmapped objects.",
      noMappedNoPath: "No implementation path gaps.",
      noMappedWithPath: "No objects with implementation paths.",
      noPlanned: "No mapped objects without implementation paths.",
      noLanded: "No mapped objects with implementation paths.",
      noProblems: "No mapping problems.",
      unmappedAttention: "Not in repository_mapping",
      mappedAttention: "In repository_mapping",
      ruleScope: {
        global: "Global rule",
        bound: "Bound rule",
        unknown: "Rule"
      },
      filters: {
        problem: "Mapping gaps",
        planned: "Mapped no paths",
        landed: "Mapped with paths",
        unit: "Units",
        rule: "Rules"
      }
    },
    review: {
      empty: "No Specs to view.",
      emptyNav: "No Spec documents.",
      emptyDetailTitle: "No Spec documents",
      emptyDetail: "The current snapshot has no candidate or stable main Spec documents to view.",
      openSource: "Open Spec source",
      fileType: "Document group",
      object: "Project object",
      reviewTarget: "View note",
      readingFocus: "View focus",
      relationships: "Relationships",
      relationEmpty: "No relationship snapshot.",

      relation: {
        implementation: "Implementation paths",
        rule: "Rules",
        ruleFile: "Rule files",
        bound: "Bound objects",
        appendix: "Appendix files",
        evidence: "Evidence references",
        stable: "Stable baseline references",
        mapping: "Project structure reference",
        system: "Global rules reference"
      },
      types: {
        candidate: "Candidate to confirm",
        stable: "Accepted stable",
        stableRule: "Accepted rules",
        capability: "Unit design",
        rule: "Rule",
        structure: "Project structure file",
        system: "Global rules file"
      },
      states: {
        candidate: "To confirm",
        stable: "Accepted",
        stableRule: "Accepted"
      },
      docKinds: {
        main: "Main",
        appendix: "Appendix",
        evidence: "Evidence",
        evidenceAppendix: "Evidence appendix"
      },
      targets: {
        candidate: "This Spec is still being confirmed and is not the formal baseline yet.",
        stable: "This Spec is already accepted and can be read as the current formal baseline.",
        stableRule: "This shared rule Spec is already accepted and can be read as the current formal rule.",
        capability: "Whether the whole file correctly expresses this capability's current design or rules.",
        rule: "Whether the whole file correctly expresses this rule and its reuse boundary.",
        structure: "Whether the whole file correctly expresses current project structure, object boundaries, and path ownership.",
        system: "Whether the whole file correctly expresses repository-wide constraints, defaults, and exceptions."
      },
      focus: {
        candidate: "Current design, boundaries, acceptance conditions, appendices, rule references",
        stable: "Formal design, next action, appendices, evidence, rule references",
        stableRule: "Rule body, reuse boundary, bound objects",
        capability: "Responsibility boundary, inputs and outputs, error handling, acceptance conditions, rule references",
        rule: "Reusing objects, rule body, binding relationships, whether it remains a local rule",
        structure: "Unit list, rule list, path ownership, support-file boundary",
        system: "Technical baseline, defaults, reusable mechanisms, prohibitions, exceptions"
      }
    },

    inspector: {
      infoTab: "Node Info",
      truthTab: "Spec Document",
      truthTitle: "Spec Document",
       fields: {
        type: "Type",
        status: "Status",
        version: "Version",
        responsibility: "Responsibility",
        file: "File",
        connections: "Connections",
        paths: "Paths"
      },
      groups: {
        truth: "Spec documents",
        implementation: "Implementation paths",
        rule: "Rules",
        bound: "Bound objects",
        connected: "Connected nodes"
      }
    },
    docMode: {
      rendered: "Rendered",
      raw: "Source"
    },
    diff: {
      show: "Show diff",
      hide: "Hide diff",
      unavailable: "No stable baseline",
      added: "Added",
      deleted: "Deleted",
      modified: "Modified",
      context: "Context",
      summary: "Changes from stable",
      stableRange: "stable",
      candidateRange: "candidate",
      insertedLines: "{count} added",
      deletedLines: "{count} deleted",
      expand: "View full diff"
    },
    source: {
      guideTitle: "Guide",
      guideShow: "Show Guide",
      guideHide: "Hide Guide",
      guideUnavailable: "No Guide",
      noGuide: "No headings",
      emptyRendered: "Select a Spec document to view its content.",
      emptyRaw: "Select a Spec document to view its source."
    },
    kind: {
      project_root: "Repository directory",
      project_path: "Path",
      project_area: "Implementation area",
      repository_mapping: "Repository mapping file",
      status_index: "Status index",
      rule: "Global rules",
      truth_file: "Spec document"
    },
    frontmatter: {
      title: "Metadata",
      undeclared: "Not declared"
    }
  }
};

const navPanel = document.getElementById("nav-panel");
const detailPanel = document.getElementById("detail-panel");
const graphView = document.getElementById("graph-view");
const viewSummary = document.getElementById("view-summary");
const projectMeta = document.getElementById("project-meta");
const sourcePath = document.getElementById("source-path");
const sourceContent = document.getElementById("source-content");
const sourceRendered = document.getElementById("source-rendered");
const docGuide = document.getElementById("doc-guide");
const docGuideToggle = document.getElementById("doc-guide-toggle");
const docDiffToggle = document.getElementById("doc-diff-toggle");
const resizeBar = document.getElementById("resize-bar");
const infoTab = document.getElementById("info-tab");
const truthTab = document.getElementById("truth-tab");
const truthPanel = document.getElementById("truth-panel");
const languageSelect = document.getElementById("language-select");

document.getElementById("refresh-button").addEventListener("click", refreshReader);
languageSelect.value = currentLanguage;
languageSelect.addEventListener("change", () => setLanguage(languageSelect.value));
document.querySelectorAll(".tab").forEach((button) => {
  button.addEventListener("click", () => {
    if (currentView === button.dataset.view) return;
    currentView = button.dataset.view;
    document.querySelectorAll(".tab").forEach((item) => item.classList.toggle("active", item === button));
    render();
  });
});

document.querySelectorAll("[data-inspector-tab]").forEach((button) => {
  button.addEventListener("click", () => {
    if (button.classList.contains("hidden")) return;
    setInspectorTab(button.dataset.inspectorTab);
  });
});

document.querySelectorAll("[data-doc-mode]").forEach((button) => {
  button.addEventListener("click", () => setDocMode(button.dataset.docMode));
});
docGuideToggle.addEventListener("click", () => setDocGuideOpen(!docGuideOpen));
docDiffToggle.addEventListener("click", () => setDiffMarkersEnabled(!diffMarkersEnabled));

resizeBar.addEventListener("pointerdown", startInspectorResize);

function readStoredLanguage() {
  try {
    const stored = window.localStorage.getItem(LANGUAGE_STORAGE_KEY);
    if (SUPPORTED_LANGUAGES.includes(stored)) return stored;
  } catch {
    return "zh-CN";
  }
  return "zh-CN";
}

function setLanguage(language) {
  currentLanguage = SUPPORTED_LANGUAGES.includes(language) ? language : "zh-CN";
  languageSelect.value = currentLanguage;
  document.documentElement.lang = currentLanguage;
  try {
    window.localStorage.setItem(LANGUAGE_STORAGE_KEY, currentLanguage);
  } catch {
    // Browser storage can be unavailable in restricted contexts.
  }
  applyStaticText();
  render();
}

function t(key, params = {}) {
  const primary = lookupTranslation(TRANSLATIONS[currentLanguage], key);
  const fallback = lookupTranslation(TRANSLATIONS["zh-CN"], key);
  const template = primary ?? fallback ?? key;
  return String(template).replace(/\{([A-Za-z0-9_]+)\}/g, (_, name) => {
    return Object.prototype.hasOwnProperty.call(params, name) ? params[name] : "";
  });
}

function lookupTranslation(source, key) {
  return String(key).split(".").reduce((value, part) => {
    if (value && Object.prototype.hasOwnProperty.call(value, part)) return value[part];
    return undefined;
  }, source);
}

function applyStaticText() {
  document.documentElement.lang = currentLanguage;
  document.querySelectorAll("[data-i18n]").forEach((element) => {
    element.textContent = t(element.dataset.i18n);
  });
  document.querySelectorAll("[data-i18n-attr]").forEach((element) => {
    element.dataset.i18nAttr.split(";").forEach((item) => {
      const [attribute, key] = item.split(":");
      if (attribute && key) element.setAttribute(attribute, t(key));
    });
  });
  if (!sourcePath.textContent) {
    sourceRendered.textContent = t("source.emptyRendered");
    sourceContent.textContent = t("source.emptyRaw");
  }
  renderDocGuide(activeSourceHeadings);
  bindDocGuideLinks();
  updateDocGuideToggle();
  updateDiffToggle();
}

async function loadSnapshot() {
  if (snapshotRequestInFlight) return;
  snapshotRequestInFlight = true;
  try {
    const response = await fetch("/api/snapshot");
    const nextSnapshot = await response.json();
    const nextSignature = snapshotSignature(nextSnapshot);
    if (!snapshot || nextSignature !== snapshotDataSignature) {
      snapshot = nextSnapshot;
      snapshotDataSignature = nextSignature;
      render();
    }
  } finally {
    snapshotRequestInFlight = false;
  }
}

async function refreshReader() {
  const openPath = sourcePath.textContent.trim();
  await loadSnapshot();
  if (openPath && isReadableOriginalPath(openPath)) {
    await openSource(openPath, { activate: activeInspectorTab === "truth" });
  }
}

function snapshotSignature(value) {
  if (!value) return "";
  const comparable = { ...value };
  delete comparable.version;
  delete comparable.generated_at;
  return JSON.stringify(comparable);
}

function render() {
  if (!snapshot) return;
  document.body.classList.toggle("spec-view-active", currentView === "spec");
  document.body.classList.toggle("registry-view-active", currentView === "registry");
  const objects = list(snapshot.objects);
  projectMeta.textContent = `${snapshot.project.repo_root} · version ${snapshot.version} · ${t("counts.objects", { count: objects.length })}`;
  const graph = graphForCurrentView();
  if (!selectedNodeID || !nodeExistsForGraph(selectedNodeID, graph)) {
    selectedNodeID = firstNodeIDForView(graph.nodes);
  }
  renderViewSummary();
  renderNav();
  renderGraph();
  renderDetailForNode(selectedNodeID);
}

function renderViewSummary() {
  const viewKey = `views.${currentView}`;
  viewSummary.innerHTML = `
    <div>
      <h2>${escapeHTML(t(`${viewKey}.title`))}</h2>
      <p>${escapeHTML(t(`${viewKey}.summary`))}</p>
    </div>
    <div class="view-counts">
      <span>${escapeHTML(t("counts.unit", { count: snapshot.project.unit_count || 0 }))}</span>
      <span>${escapeHTML(t("counts.rule", { count: snapshot.project.rule_count || 0 }))}</span>
      <span>${escapeHTML(t("counts.truth", { count: snapshot.project.truth_file_count || 0 }))}</span>
    </div>
  `;
}

function renderNav() {
  navPanel.innerHTML = "";
  const navTitle = document.createElement("div");
  navTitle.className = "nav-title";
  navTitle.textContent = t(`views.${currentView}.nav`);
  navPanel.appendChild(navTitle);

  const diagnostics = list(snapshot.diagnostics);
  if (diagnostics.length > 0) {
    diagnostics.forEach((diagnostic) => {
      const div = document.createElement("div");
      div.className = "diagnostic";
      div.textContent = `${diagnostic.severity}: ${diagnostic.message}`;
      navPanel.appendChild(div);
    });
  }

  if (currentView === "project") {
    const graph = graphForCurrentView();
    const objects = graph.nodes
      .filter((node) => (node.group === "unit" || node.group === "rule") && list(node.raw_paths).length > 0)
      .sort(byLabel);
    objects.forEach((node) => {
      const button = document.createElement("button");
      button.className = node.id === selectedNodeID ? "nav-item active" : "nav-item";
      button.type = "button";
      button.innerHTML = `<strong>${escapeHTML(node.label)}</strong><span>${escapeHTML(t("counts.paths", { count: list(node.raw_paths).length }))}</span>`;
      button.addEventListener("click", () => focusNode(node.id));
      navPanel.appendChild(button);
    });
    return;
  }

  if (currentView === "specflow") {
    renderSpecflowNav();
    return;
  }

  if (currentView === "registry") {
    renderRegistryNav();
    return;
  }

  if (currentView === "spec") {
    renderReviewNav();
    return;
  }

  const objects = objectsForView();
  objects.forEach((object) => {
    const button = document.createElement("button");
    button.className = objectNodeID(object) === selectedNodeID ? "nav-item active" : "nav-item";
    button.type = "button";
    button.innerHTML = `<strong>${escapeHTML(object.label)}</strong><span>${escapeHTML(navSubtitle(object))}</span>`;
    button.addEventListener("click", () => focusObject(object));
    navPanel.appendChild(button);
  });
}

function renderSpecflowNav() {
  const graph = graphForSpecflowView();
  const objects = list(snapshot.objects);
  const units = objects.filter((item) => item.kind === "unit").sort(byLabel);
  const rules = objects.filter((item) => item.kind === "rule").sort(byLabel);
  const truthNodes = graph.nodes.filter((node) => node.group === "truth").sort(byLabel);
  const implementationNodes = graph.nodes.filter((node) => node.group === "implementation").sort(byLabel);
  const systemNodes = graph.nodes.filter((node) => node.group === "__unused_rule_group__").sort(byLabel);
  const supportNodes = graph.nodes.filter((node) => node.group === "support").sort(byLabel);

  const sections = [
    { key: "unit", type: "objects", items: units },
    { key: "rule", type: "objects", items: rules },
    { key: "truth", type: "nodes", items: truthNodes },
    { key: "implementation", type: "nodes", items: implementationNodes },
    { key: "system", type: "nodes", items: systemNodes },
    { key: "support", type: "nodes", items: supportNodes }
  ].filter((section) => section.items.length > 0);
  if (!sections.some((section) => section.key === activeSpecflowNavGroup)) {
    activeSpecflowNavGroup = (sections[0] || {}).key || "unit";
  }
  sections.forEach((section) => {
    if (section.type === "objects") {
      renderObjectNavSection(section.key, section.items);
      return;
    }
    renderNodeNavSection(section.key, section.items);
  });
}

function renderReviewNav() {
  const items = reviewItems();
  const sections = reviewTypeOrder()
    .map((type) => ({ type, groups: reviewObjectGroups(items.filter((item) => item.reviewType === type)) }))
    .filter((section) => section.groups.length > 0);
  if (sections.length === 0) {
    const empty = document.createElement("div");
    empty.className = "nav-empty";
    empty.textContent = t("review.emptyNav");
    navPanel.appendChild(empty);
    return;
  }
  if (!sections.some((section) => section.type === activeReviewNavGroup)) {
    activeReviewNavGroup = (sections[0] || {}).type || "capability";
  }
  sections.forEach((section) => renderReviewNavSection(section.type, section.groups));
}

function renderReviewNavSection(type, groups) {
  const expanded = type === activeReviewNavGroup;
  const section = document.createElement("section");
  section.className = expanded ? "nav-section expanded" : "nav-section";

  const header = document.createElement("button");
  header.className = "nav-section-title";
  header.type = "button";
  header.setAttribute("aria-expanded", String(expanded));
  header.innerHTML = `<span>${escapeHTML(reviewTypeLabel(type))}</span><em>${groups.length}</em>`;
  header.addEventListener("click", () => {
    activeReviewNavGroup = type;
    renderNav();
  });
  section.appendChild(header);

  if (expanded) {
    ensureReviewSectionExpansion(type, groups);
    groups.forEach((group) => {
      const groupExpanded = expandedReviewObjectKeys.has(group.key);
      const objectButton = document.createElement("button");
      objectButton.className = `nav-object ${objectKindClass(group.object.kind)}${groupExpanded ? " expanded" : ""}`;
      objectButton.type = "button";
      objectButton.title = group.objectLabel;
      objectButton.setAttribute("aria-expanded", String(groupExpanded));
      objectButton.innerHTML = `${renderNavItemTitle(group.objectLabel, group.object.kind)}<em>${group.items.length}</em>`;
      objectButton.addEventListener("click", () => {
        initializedReviewNavGroups.add(type);
        if (expandedReviewObjectKeys.has(group.key)) {
          expandedReviewObjectKeys.delete(group.key);
        } else {
          expandedReviewObjectKeys.add(group.key);
        }
        renderNav();
      });
      section.appendChild(objectButton);

      if (!groupExpanded) return;
      const fileList = document.createElement("div");
      fileList.className = "review-file-list";
      group.items.forEach((item) => {
        const button = document.createElement("button");
        button.className = `nav-item review-file-item ${objectKindClass(item.object.kind)}${item.id === selectedNodeID ? " active" : ""}`;
        button.type = "button";
        button.title = `${item.fileLabel}\n${item.path}`;
        button.innerHTML = `<strong>${escapeHTML(reviewTreeFileLabel(item))}</strong><span>${escapeHTML(reviewTreeFileSubtitle(item))}</span>`;
        button.addEventListener("click", () => focusReviewItem(item.id));
        fileList.appendChild(button);
      });
      section.appendChild(fileList);
    });
  }
  navPanel.appendChild(section);
}

function renderObjectNavSection(sectionKey, objects) {
  if (!objects || objects.length === 0) return;
  const section = createNavSection(sectionKey, objects.length);
  if (sectionKey !== activeSpecflowNavGroup) {
    navPanel.appendChild(section);
    return;
  }
  objects.forEach((object) => {
    const button = document.createElement("button");
    button.className = objectNodeID(object) === selectedNodeID ? "nav-item active" : "nav-item";
    button.type = "button";
    button.innerHTML = `<strong>${escapeHTML(object.label)}</strong><span>${escapeHTML(navSubtitle(object))}</span>`;
    button.addEventListener("click", () => focusObject(object));
    section.appendChild(button);
  });
  navPanel.appendChild(section);
}

function renderNodeNavSection(sectionKey, nodes) {
  if (!nodes || nodes.length === 0) return;
  const section = createNavSection(sectionKey, nodes.length);
  if (sectionKey !== activeSpecflowNavGroup) {
    navPanel.appendChild(section);
    return;
  }
  nodes.forEach((node) => {
    const button = document.createElement("button");
    button.className = node.id === selectedNodeID ? "nav-item active" : "nav-item";
    button.type = "button";
    button.innerHTML = `<strong>${escapeHTML(compactLabel(node))}</strong><span>${escapeHTML(node.source && node.source.path ? node.source.path : labelForKind(node.kind))}</span>`;
    button.addEventListener("click", () => focusNode(node.id));
    section.appendChild(button);
  });
  navPanel.appendChild(section);
}

function createNavSection(sectionKey, count) {
  const section = document.createElement("section");
  const expanded = sectionKey === activeSpecflowNavGroup;
  section.className = expanded ? "nav-section expanded" : "nav-section";
  const header = document.createElement("button");
  header.className = "nav-section-title";
  header.type = "button";
  header.setAttribute("aria-expanded", String(expanded));
  header.innerHTML = `<span>${escapeHTML(t(`specflowSections.${sectionKey}`))}</span><em>${count}</em>`;
  header.addEventListener("click", () => {
    activeSpecflowNavGroup = sectionKey;
    renderNav();
  });
  section.appendChild(header);
  return section;
}

function objectsForView() {
  const objects = list(snapshot.objects);
  if (currentView === "project") {
    return objects.filter((item) => item.kind === "unit");
  }
  if (currentView === "specflow") {
    return objects.filter((item) => item.kind === "rule").concat(objects.filter((item) => item.kind === "unit"));
  }
  return objects;
}

function navSubtitle(object) {
  if (currentView === "specflow") {
    return object.responsibility || t("fallback.responsibilityUnknown");
  }
  if (object.kind === "rule") return object.responsibility || t("fallback.rule");
  return object.kind;
}

function focusObject(object) {
  selectedNodeID = objectNodeID(object);
  renderNav();
  renderDetailForNode(selectedNodeID);
  focusGraphNode(selectedNodeID, 1.05);
}

function focusNode(nodeID) {
  selectedNodeID = nodeID;
  renderNav();
  renderDetailForNode(nodeID);
  focusGraphNode(nodeID, 1.05);
}

function focusGraphNode(nodeID, zoom) {
  if (!cy || !nodeID) return;
  const node = cy.getElementById(nodeID);
  if (node.length > 0) {
    cy.elements().removeClass("selected");
    node.addClass("selected");
    cy.animate({ center: { eles: node }, zoom }, { duration: 250 });
  }
}

function renderGraph() {
  if (currentView === "spec") {
    if (cy) {
      cy.destroy();
      cy = null;
    }
    renderReviewBoard();
    return;
  }
  if (currentView === "registry") {
    if (cy) {
      cy.destroy();
      cy = null;
    }
    renderRegistryBoard();
    return;
  }
  if (typeof cytoscape !== "function") {
    graphView.textContent = t("fallback.cytoscapeMissing");
    return;
  }
  const graph = graphForCurrentView();
  const preserveGraphViewport = cy && lastGraphView === currentView
    ? { pan: { ...cy.pan() }, zoom: cy.zoom(), selectedNodeID }
    : null;
  if (cy) {
    cy.destroy();
    cy = null;
  }
  graphView.innerHTML = "";
  const positions = readablePositions(graph.nodes, graph.edges);
  const elements = [];
  graph.nodes.forEach((node) => {
    elements.push({
      data: {
        id: node.id,
        label: compactLabel(node),
        kind: node.kind,
        group: node.group,
        source: node.source || null
      },
      position: positions[node.id] || { x: 100, y: 100 }
    });
  });
  graph.edges.forEach((edge) => {
    elements.push({
      data: {
        id: edge.id,
        source: edge.from,
        target: edge.to,
        label: edgeLabel(edge.kind),
        kind: edge.kind,
        sourceRef: edge.source || null
      }
    });
  });
  cy = cytoscape({
    container: graphView,
    elements,
    style: [
      { selector: "node", style: {
        "label": "data(label)",
        "text-wrap": "wrap",
        "text-max-width": 150,
        "font-size": 12,
        "color": "#1f2933",
        "text-halign": "right",
        "text-valign": "center",
        "text-margin-x": 11,
        "text-margin-y": 0,
        "background-color": colorForGroup,
        "border-width": 1,
        "border-color": "#ffffff",
        "width": nodeSize,
        "height": nodeSize,
        "text-background-color": "#ffffff",
        "text-background-opacity": 0.86,
        "text-background-padding": 2
      }},
      { selector: 'node[group = "implementation"]', style: {
        "text-halign": "left",
        "text-margin-x": -11
      }},
      { selector: "edge", style: {
        "curve-style": "bezier",
        "target-arrow-shape": "triangle",
        "target-arrow-color": "#94a3b8",
        "line-color": "#94a3b8",
        "width": edgeWidth,
        "label": "",
        "opacity": 0.7
      }},
      { selector: ".selected", style: {
        "border-width": 5,
        "border-color": "#2563eb"
      }},
      { selector: ".connected", style: {
        "line-color": "#2563eb",
        "target-arrow-color": "#2563eb",
        "opacity": 1,
        "width": 2.4
      }}
    ],
    minZoom: 0.35,
    maxZoom: 2.2,
    wheelSensitivity: 1.1,
    layout: { name: "preset", fit: false }
  });
  cy.ready(() => {
    if (preserveGraphViewport && nodeExistsForGraph(preserveGraphViewport.selectedNodeID, graph)) {
      cy.zoom(preserveGraphViewport.zoom);
      cy.pan(preserveGraphViewport.pan);
      highlightConnected(selectedNodeID);
      return;
    }
    focusGraphNode(selectedNodeID || firstNodeIDForView(graph.nodes), 0.85);
  });
  lastGraphView = currentView;
  cy.on("tap", "node", (event) => {
    const data = event.target.data();
    selectedNodeID = data.id;
    highlightConnected(data.id);
    renderNav();
    renderDetailForNode(data.id);
  });
  cy.on("tap", "edge", (event) => {
    const source = event.target.data("sourceRef");
    if (source && isReadableOriginalPath(source.path)) openSource(source.path);
  });
}

function graphForCurrentView() {
  if (currentView === "spec") return graphForReviewView();
  if (currentView === "project") return graphForProjectView();
  if (currentView === "specflow") return graphForSpecflowView();
  if (currentView === "registry") return graphForRegistryView();

  const nodes = list(snapshot.nodes);
  const edges = list(snapshot.edges);
  return { nodes, edges };
}

function graphForRegistryView() {
  return {
    nodes: registryItems().map((item) => ({
      id: registryNodeID(item),
      kind: item.kind,
      label: item.label || item.id,
      group: item.result || "gap",
      source: firstSourceRef(item.sources)
    })),
    edges: []
  };
}

function graphForReviewView() {
  return {
    nodes: reviewItems().map((item) => ({
      id: item.id,
      kind: item.reviewType,
      label: item.fileLabel,
      group: item.reviewType,
      source: item.source
    })),
    edges: []
  };
}

function graphForProjectView() {
  const nodesByID = new Map();
  const edges = [];
  const rootsByID = new Map();
  const areasByID = new Map();
  const addNode = (node) => {
    if (!nodesByID.has(node.id)) nodesByID.set(node.id, node);
  };
  const updateNode = (node) => {
    nodesByID.set(node.id, { ...(nodesByID.get(node.id) || {}), ...node });
  };
  const addEdge = (edge) => {
      if (!edges.some((item) => item.id === edge.id)) edges.push(edge);
  };

  list(snapshot.objects).forEach((object) => {
    const implementationPaths = list(object.implementation_paths);
    if (implementationPaths.length === 0) return;
    const objectID = objectNodeID(object);
    addNode({
      id: objectID,
      kind: object.kind,
      label: object.label,
      group: object.kind === "rule" ? "rule" : object.kind,
      source: firstSourceRef(object.sources),
      raw_paths: implementationPaths
    });
    implementationPaths.forEach((ref) => {
      const pathID = addProjectArea(addNode, addEdge, rootsByID, areasByID, ref);
      addEdge({
        id: `${pathID}->${objectID}`,
        from: pathID,
        to: objectID,
        kind: "maps_to",
        label: "maps to",
        source: ref
      });
    });
  });

  areasByID.forEach((area) => {
    updateNode({
      id: area.id,
      label: `${area.aggregate_path_label} · ${t("counts.paths", { count: area.raw_paths.length })}`,
      aggregate_path_count: area.raw_paths.length,
      raw_paths: area.raw_paths
    });
  });
  rootsByID.forEach((root) => {
    updateNode({
      id: root.id,
      aggregate_path_count: root.raw_paths.length,
      raw_paths: root.raw_paths
    });
  });

  return { nodes: [...nodesByID.values()], edges };
}

function addProjectArea(addNode, addEdge, rootsByID, areasByID, ref) {
  if (!ref || !ref.path) return "";
  const root = rootForImplementationPath(ref.path);
  if (!rootsByID.has(root.id)) {
    root.raw_paths = [];
    rootsByID.set(root.id, root);
    addNode(root);
  }
  rootsByID.get(root.id).raw_paths.push(ref);

  const areaPath = projectAreaForImplementationPath(ref.path);
  const pathID = `project_area:${areaPath}`;
  if (!areasByID.has(pathID)) {
    const area = {
      id: pathID,
      kind: "project_area",
      label: areaPath,
      group: "implementation",
      source: sourceForImplementationRef(ref),
      aggregate_path_label: areaPath,
      aggregate_path_count: 0,
      raw_paths: []
    };
    areasByID.set(pathID, area);
    addNode(area);
  }
  areasByID.get(pathID).raw_paths.push(ref);
  addEdge({
    id: `${root.id}->${pathID}`,
    from: root.id,
    to: pathID,
    kind: "contains",
    label: "contains",
    source: ref
  });
  return pathID;
}

function projectAreaForImplementationPath(rawPath) {
  const path = String(rawPath || "").replaceAll("\\", "/").replace(/\/\*\*$/, "/");
  if (path.endsWith("/")) return path;
  const segments = path.split("/");
  if (segments.length <= 1) return path;
  segments.pop();
  return `${segments.join("/")}/`;
}

function rootForImplementationPath(path) {
  const firstSegment = String(path).split("/")[0] || "unknown";
  const label = path.includes("/") ? `${firstSegment}/` : firstSegment;
  return { id: `root:${label}`, kind: "project_root", label, group: "root" };
}

function graphForSpecflowView() {
  const nodesByID = new Map();
  const edges = [];
  const addNode = (node) => {
    if (!nodesByID.has(node.id)) nodesByID.set(node.id, node);
  };
  const addEdge = (edge) => {
    if (!edges.some((item) => item.id === edge.id)) edges.push(edge);
  };

  addNode({ id: "rule:baseline", kind: "rule", label: t("kind.rule"), group: "rule", source: { path: snapshot.project.rule_baseline_file } });
  addNode({ id: "support:repository_mapping", kind: "repository_mapping", label: t("kind.repository_mapping"), group: "support", source: { path: snapshot.project.mapping_file } });
  addNode({ id: "support:status", kind: "status_index", label: t("kind.status_index"), group: "support", source: { path: snapshot.project.status_file } });
  addEdge({ id: "rule:baseline->support:repository_mapping", from: "rule:baseline", to: "support:repository_mapping", kind: "constrains", label: "constrains", source: { path: snapshot.project.rule_baseline_file } });

  list(snapshot.objects).forEach((object) => {
    const objectID = objectNodeID(object);
    addNode({
      id: objectID,
      kind: object.kind,
      label: object.label,
      group: object.kind === "rule" ? "rule" : object.kind,
      source: firstSourceRef(object.sources)
    });
    addEdge({ id: `support:repository_mapping->${objectID}`, from: "support:repository_mapping", to: objectID, kind: "declares", label: "declares", source: { path: snapshot.project.mapping_file } });
    if (object.kind === "unit") {
      addEdge({ id: `support:status->${objectID}`, from: "support:status", to: objectID, kind: "tracks_state", label: "tracks state", source: { path: snapshot.project.status_file } });
    }
    list(object.truth_paths).forEach((truth) => {
      const fileNode = `file:${truth.path}`;
      addNode({ id: fileNode, kind: "truth_file", label: truth.path.split("/").pop(), group: "truth", source: truth });
      addEdge({ id: `${objectID}->${fileNode}`, from: objectID, to: fileNode, kind: "described_by", label: "described by", source: truth });
    });
    list(object.rule_refs).forEach((ruleID) => {
      const ruleNode = `rule:${ruleID}`;
      addNode({ id: ruleNode, kind: "rule", label: ruleID, group: "rule" });
      addEdge({ id: `${objectID}->${ruleNode}`, from: objectID, to: ruleNode, kind: "uses_rule", label: "uses rule", source: firstSourceRef(object.sources) });
    });
    list(object.bound_objects).forEach((bound) => {
      if (object.kind !== "rule") return;
      addEdge({ id: `${objectID}->${bound}`, from: objectID, to: bound, kind: "bound_to", label: "bound to", source: firstSourceRef(object.sources) });
    });
  });

  return { nodes: [...nodesByID.values()], edges };
}

function firstSourceRef(sources) {
  return list(sources).find((source) => source && source.path) || null;
}

function readablePositions(nodes, edges) {
  if (currentView === "project") return projectPositions(nodes, edges);
  if (currentView === "specflow") return specflowPositions(nodes, edges);
  return relationshipPositions(nodes, edges);
}

function projectPositions(nodes, edges) {
  const positions = {};
  const roots = nodes.filter((node) => node.group === "root").sort(byLabel);
  const areas = nodes.filter((node) => node.kind === "project_area").sort(byLabel);
  const objects = nodes
    .filter((node) => node.kind !== "project_area" && node.group !== "root")
    .sort(byLabel);
  const top = 80;
  const areaGap = 68;
  const rootGap = 150;
  const objectGap = 116;
  const rootX = 120;
  const areaX = 520;
  const objectX = 860;

  objects.forEach((node, index) => {
    positions[node.id] = { x: objectX, y: top + index * objectGap };
  });

  areas.forEach((node, index) => {
    const owners = edges
      .filter((edge) => edge.from === node.id && positions[edge.to])
      .map((edge) => positions[edge.to].y);
    positions[node.id] = { x: areaX, y: owners.length > 0 ? average(owners) : top + index * areaGap };
  });
  distributeColumn(areas, positions, areaX, top, areaGap);

  roots.forEach((node, index) => {
    const children = edges
      .filter((edge) => edge.from === node.id && positions[edge.to])
      .map((edge) => positions[edge.to].y);
    positions[node.id] = { x: rootX, y: children.length > 0 ? average(children) : top + index * rootGap };
  });
  distributeColumn(roots, positions, rootX, top, rootGap);
  return positions;
}

function specflowPositions(nodes, edges) {
  const positions = {};
  const groups = {
    system: nodes.filter((node) => node.group === "support"),
    rule: nodes.filter((node) => node.group === "rule"),
    domain: nodes.filter((node) => node.group === "unit"),
    truth: nodes.filter((node) => node.group === "truth")
  };
  const x = { system: 120, rule: 390, domain: 650, truth: 940 };
  const top = 90;

  groups.system.sort(byLabel).forEach((node, index) => {
    positions[node.id] = { x: x.system, y: top + index * 110 };
  });
  groups.rule.sort(byLabel).forEach((node, index) => {
    positions[node.id] = { x: x.rule, y: top + index * 118 };
  });
  groups.domain.sort(byLabel).forEach((node, index) => {
    positions[node.id] = { x: x.domain, y: top + index * 118 };
  });
  positionChildGroup(groups.truth, edges, positions, x.truth, top, 72);
  distributeColumn(groups.truth, positions, x.truth, top, 60);
  return positions;
}

function relationshipPositions(nodes, edges) {
  const positions = {};
  const groups = {
    rule: nodes.filter((node) => node.group === "rule"),
    domain: nodes.filter((node) => node.group === "unit"),
    truth: nodes.filter((node) => node.group === "truth"),
    implementation: nodes.filter((node) => node.group === "implementation"),
    system: nodes.filter((node) => node.group === "support")
  };
  const x = { rule: 120, system: 120, domain: 430, truth: 760, implementation: 1100 };
  const rowGap = 135;
  const top = 90;

  groups.domain.sort(byLabel).forEach((node, index) => {
    positions[node.id] = { x: x.domain, y: top + index * rowGap };
  });
  groups.system.sort(byLabel).forEach((node, index) => {
    positions[node.id] = { x: x.system, y: top + index * rowGap };
  });

  groups.rule.sort(byLabel).forEach((node, index) => {
    const boundTargets = edges
      .filter((edge) => edge.from === node.id && positions[edge.to])
      .map((edge) => positions[edge.to].y);
    const y = boundTargets.length > 0 ? average(boundTargets) : top + index * rowGap;
    positions[node.id] = { x: x.rule, y };
  });

  distributeColumn(groups.rule.concat(groups.system), positions, x.rule, top, 112);
  positionChildGroup(groups.truth, edges, positions, x.truth, top, 72);
  positionChildGroup(groups.implementation, edges, positions, x.implementation, top, 82);

  nodes.forEach((node, index) => {
    if (!positions[node.id]) {
      positions[node.id] = { x: 430, y: top + index * rowGap };
    }
  });
  return positions;
}

function positionChildGroup(nodes, edges, positions, x, fallbackTop, gap) {
  const childrenByParent = new Map();
  nodes.sort(byLabel).forEach((node) => {
    const parentEdge = edges.find((edge) => edge.to === node.id && positions[edge.from]);
    const parent = parentEdge ? parentEdge.from : "";
    if (!childrenByParent.has(parent)) childrenByParent.set(parent, []);
    childrenByParent.get(parent).push(node);
  });
  let nextY = fallbackTop;
  [...childrenByParent.entries()]
    .map(([parent, children]) => {
      const parentY = parent ? positions[parent].y : fallbackTop;
      return { parent, children, parentY };
    })
    .sort((left, right) => left.parentY - right.parentY || String(left.parent).localeCompare(String(right.parent)))
    .forEach(({ children, parentY }) => {
      const centeredStart = parentY - ((children.length - 1) * gap) / 2;
      const startY = Math.max(centeredStart, nextY);
      children.forEach((node, index) => {
        positions[node.id] = { x, y: startY + index * gap };
      });
      nextY = startY + children.length * gap;
    });
}

function distributeColumn(nodes, positions, x, fallbackTop, gap) {
  let nextY = fallbackTop;
  nodes
    .filter((node) => positions[node.id])
    .sort((left, right) => positions[left.id].y - positions[right.id].y || byLabel(left, right))
    .forEach((node) => {
      const y = Math.max(positions[node.id].y, nextY);
      positions[node.id] = { x, y };
      nextY = y + gap;
    });
}

function highlightConnected(nodeID) {
  if (!cy) return;
  cy.elements().removeClass("selected connected");
  const node = cy.getElementById(nodeID);
  node.addClass("selected");
  node.connectedEdges().addClass("connected");
}

function colorForGroup(ele) {
  const group = ele.data("group");
  if (group === "unit") return "#2563eb";
  if (group === "rule") return "#0f766e";
  if (group === "truth") return "#7c3aed";
  if (group === "implementation") return "#b45309";
  if (group === "root") return "#0f172a";
  if (group === "support") return "#64748b";
  return "#475569";
}

function objectFromNode(id) {
  if (!id.includes(":")) return null;
  const [kind, objectID] = id.split(":", 2);
  const objectKind = kind === "rule" ? "rule" : kind;
  return list(snapshot.objects).find((item) => item.kind === objectKind && item.id === objectID);
}

function objectNodeID(object) {
  if (!object) return null;
  return `${object.kind === "rule" ? "rule" : object.kind}:${object.id}`;
}

function nodeExistsForGraph(nodeID, graph) {
  return list(graph.nodes).some((node) => node.id === nodeID);
}

function firstNodeIDForView(nodes) {
  if (currentView === "spec") {
    return (nodes[0] || {}).id || null;
  }
  if (currentView === "project") {
    const objectNode = nodes.find((node) => (node.group === "unit" || node.group === "rule") && list(node.raw_paths).length > 0);
    return (objectNode || nodes[0] || {}).id || null;
  }
  if (currentView === "specflow") {
    const supportNode = nodes.find((node) => node.id === "rule:baseline");
    return (supportNode || nodes[0] || {}).id || null;
  }
  if (currentView === "registry") {
    return (nodes[0] || {}).id || null;
  }
  const domainNode = nodes.find((node) => node.group === "unit");
  return (domainNode || nodes[0] || {}).id || null;
}

function renderFlag(value) {
  const active = yesish(value);
  return `<span class="flag ${active ? "flag-yes" : "flag-no"}">${escapeHTML(value || "no")}</span>`;
}

function renderSourceButton(path, label) {
  if (!path) return "";
  return `<button class="source-link" type="button" data-source="${escapeAttr(path)}">${escapeHTML(label)}</button>`;
}

function yesish(value) {
  return String(value || "").toLowerCase() === "yes";
}

function registryItems() {
  return list(snapshot.registry).slice().sort((left, right) => {
    const resultOrder = registryResultOrder(left.result) - registryResultOrder(right.result);
    if (resultOrder !== 0) return resultOrder;
    if (left.kind !== right.kind) return String(left.kind || "").localeCompare(String(right.kind || ""));
    return String(left.id || "").localeCompare(String(right.id || ""));
  });
}

function registryResultOrder(result) {
  if (registryProblemResult(result)) return 0;
  if (result === "planned") return 1;
  if (result === "landed") return 2;
  return 3;
}

function registryNodeID(item) {
  return `registry:${item.kind}:${item.id}`;
}

function registryItemByID(itemID) {
  return registryItems().find((item) => registryNodeID(item) === itemID) || null;
}

function renderRegistryNav() {
  const items = registryItems();
  const sections = [
    { key: "problem", items: items.filter((item) => registryProblemResult(item.result)) },
    { key: "planned", items: items.filter((item) => item.result === "planned") },
    { key: "landed", items: items.filter((item) => item.result === "landed") },
    { key: "unit", items: items.filter((item) => item.kind === "unit") },
    { key: "rule", items: items.filter((item) => item.kind === "rule") }
  ].filter((section) => section.items.length > 0);
  if (sections.length === 0) {
    const empty = document.createElement("div");
    empty.className = "nav-empty";
    empty.textContent = t("fallback.noObject");
    navPanel.appendChild(empty);
    return;
  }
  if (!sections.some((section) => section.key === activeRegistryNavGroup)) {
    activeRegistryNavGroup = (sections[0] || {}).key || "problem";
  }
  sections.forEach((section) => renderRegistryNavSection(section.key, section.items));
}

function renderRegistryNavSection(sectionKey, items) {
  const expanded = sectionKey === activeRegistryNavGroup;
  const section = document.createElement("section");
  section.className = expanded ? "nav-section expanded" : "nav-section";

  const header = document.createElement("button");
  header.className = "nav-section-title";
  header.type = "button";
  header.setAttribute("aria-expanded", String(expanded));
  header.innerHTML = `<span>${escapeHTML(t(`registry.filters.${sectionKey}`))}</span><em>${items.length}</em>`;
  header.addEventListener("click", () => {
    activeRegistryNavGroup = sectionKey;
    renderNav();
  });
  section.appendChild(header);

  if (expanded) {
    items.forEach((item) => {
      const button = document.createElement("button");
      button.className = `nav-item ${objectKindClass(item.kind)}${registryNodeID(item) === selectedNodeID ? " active" : ""}`;
      button.type = "button";
      button.innerHTML = `
        <span class="nav-item-title">
          ${renderRegistryKindBadge(item)}
          <strong>${escapeHTML(item.label || item.id)}</strong>
        </span>
        <span>${escapeHTML(registryResultLabel(item.result))}</span>
      `;
      button.addEventListener("click", () => focusNode(registryNodeID(item)));
      section.appendChild(button);
    });
  }
  navPanel.appendChild(section);
}

function renderRegistryBoard() {
  const items = registryItems();
  const missingMappingItems = items.filter((item) => item.result === "unregistered_file");
  const mappedNoPathItems = items.filter((item) => item.result === "planned" || item.result === "missing_file" || item.result === "invalid_registry_row");
  const mappedWithPathItems = items.filter((item) => item.result === "landed");
  graphView.innerHTML = `
    <section class="registry-board">
      <section class="registry-section">
        <div class="registry-section-heading">
          <div>
            <h3>${escapeHTML(t("registry.boardHeading"))}</h3>
            <p>${escapeHTML(t("registry.boardDescription"))}</p>
          </div>
          ${renderSourceButton(snapshot.project.mapping_file, t("registry.mappingSource"))}
        </div>
        ${renderRegistryMetrics(items, missingMappingItems, mappedNoPathItems, mappedWithPathItems)}
        ${renderRegistrySection(t("registry.missingMappingHeading"), t("registry.missingMappingDescription"), missingMappingItems, t("registry.noMissingMapping"))}
        ${renderRegistrySection(t("registry.mappedNoPathHeading"), t("registry.mappedNoPathDescription"), mappedNoPathItems, t("registry.noMappedNoPath"))}
        ${renderRegistrySection(t("registry.mappedWithPathHeading"), t("registry.mappedWithPathDescription"), mappedWithPathItems, t("registry.noMappedWithPath"))}
      </section>
    </section>
  `;
  bindRegistryBoardLinks();
}

function renderRegistryMetrics(items, missingMappingItems, mappedNoPathItems, mappedWithPathItems) {
  const knownUnits = items.filter((item) => item.kind === "unit").length;
  const metrics = [
    { value: knownUnits, label: t("registry.knownUnits") },
    { value: missingMappingItems.length, label: t("registry.missingMapping") },
    { value: mappedNoPathItems.length, label: t("registry.mappedWithoutPath") },
    { value: mappedWithPathItems.length, label: t("registry.mappedWithPath") }
  ];
  return `
    <div class="metric-grid registry-metric-grid">
      ${metrics.map((metric) => `
        <div class="metric">
          <strong>${escapeHTML(metric.value)}</strong>
          <span>${escapeHTML(metric.label)}</span>
        </div>
      `).join("")}
    </div>
  `;
}

function renderRegistrySection(title, description, items, emptyText) {
  return `
    <section class="registry-section">
      <div class="registry-section-title">
        <div>
          <h4>${escapeHTML(title)}</h4>
          <p>${escapeHTML(description)}</p>
        </div>
        <em>${escapeHTML(items.length)}</em>
      </div>
      ${items.length > 0 ? renderRegistryTable(items) : `<p class="empty-copy registry-empty">${escapeHTML(emptyText)}</p>`}
    </section>
  `;
}

function renderRegistryTable(items) {
  return `
    <div class="registry-table-wrap">
      <table class="registry-table">
        <colgroup>
          <col style="width:70px">
          <col style="width:140px">
          <col style="width:120px">
          <col style="min-width:280px;width:auto">
          <col style="width:130px">
          <col>
        </colgroup>
        <thead>
          <tr>
            <th>${escapeHTML(t("inspector.fields.type"))}</th>
            <th>${escapeHTML(t("registry.objectLabel"))}</th>
            <th>${escapeHTML(t("registry.result"))}</th>
            <th>${escapeHTML(t("registry.implementation"))}</th>
            <th>${escapeHTML(t("registry.relation"))}</th>
            <th>${escapeHTML(t("registry.attention"))}</th>
          </tr>
        </thead>
        <tbody>${items.map(renderRegistryRow).join("")}</tbody>
      </table>
    </div>
  `;
}

function renderRegistryRow(item) {
  return `
    <tr class="registry-row ${escapeAttr(item.result || "gap")}">
      <td>${renderRegistryKindBadge(item)}</td>
      <td><button class="table-object" type="button" data-registry="${escapeAttr(registryNodeID(item))}">${escapeHTML(item.label || item.id)}</button></td>
      <td>${renderRegistryResult(item.result)}</td>
      <td>${renderRegistryImplementationPaths(item)}</td>
      <td>${escapeHTML(registryRefSummary(item))}</td>
      <td>${escapeHTML(registryAttentionSummary(item))}</td>
    </tr>
  `;
}

function renderRegistryResult(result) {
  const normalized = String(result || "invalid_registry_row");
  return `<span class="registry-result ${escapeAttr(normalized)}">${escapeHTML(registryResultLabel(normalized))}</span>`;
}

function registryResultLabel(result) {
  switch (result) {
    case "planned":
      return t("registry.planned");
    case "landed":
      return t("registry.landed");
    case "missing_file":
      return t("registry.missingFile");
    case "unregistered_file":
      return t("registry.unregisteredFile");
    case "invalid_registry_row":
      return t("registry.invalidRegistryRow");
    default:
      return result || t("registry.invalidRegistryRow");
  }
}

function registryProblemResult(result) {
  return result === "missing_file" || result === "unregistered_file" || result === "invalid_registry_row";
}

function renderRegistryPresence(registered, source) {
  const label = registered ? t("registry.yes") : t("registry.no");
  const className = registered ? "flag-yes" : "flag-no";
  if (registered && source && source.path) {
    return `<button class="flag ${className} source-flag" type="button" data-source="${escapeAttr(source.path)}">${escapeHTML(label)}</button>`;
  }
  return `<span class="flag ${className}">${escapeHTML(label)}</span>`;
}

function renderRegistryMappingPresence(item) {
  if (item.kind === "rule") {
    if (item.mapping_registered && item.mapping_source && item.mapping_source.path) {
      return `<button class="flag flag-yes source-flag" type="button" data-source="${escapeAttr(item.mapping_source.path)}" ${item.mapping_source.line ? `data-source-line="${escapeAttr(item.mapping_source.line)}"` : ""}>${escapeHTML(t("registry.declared"))}</button>`;
    }
    return `<span class="flag">${escapeHTML(t("registry.optional"))}</span>`;
  }
  return renderRegistryPresence(item.mapping_registered, item.mapping_source);
}

function renderRegistryStatusPresence(item) {
  if (item.kind === "rule") {
    return `<span class="flag">${escapeHTML(t("registry.notApplicable"))}</span>`;
  }
  return renderRegistryPresence(item.status_registered, item.status_source);
}

function registryRefSummary(item) {
  const parts = [];
  if (item.kind === "rule") {
    if (item.rule_scope === "global") return t("registry.globalActive");
    const boundObjects = list(item.bound_objects).length;
    return boundObjects > 0 ? `${t("registry.boundObjects")} ${boundObjects}` : t("registry.unboundRule");
  }
  const unitRefs = list(item.unit_refs).length;
  const ruleRefs = list(item.rule_refs).length;
  if (unitRefs > 0) parts.push(`${t("registry.unitRefs")} ${unitRefs}`);
  if (ruleRefs > 0) parts.push(`${t("registry.ruleRefs")} ${ruleRefs}`);
  return parts.length > 0 ? parts.join(" · ") : t("fallback.none");
}

function registryImplementationSummary(item) {
  const count = list(item.implementation_paths).length;
  return count > 0 ? t("counts.paths", { count }) : t("registry.no");
}

function registryEvidenceSummary(item) {
  const parts = [];
  if (item.status_registered) parts.push(t("registry.status"));
  if (item.truth_registered) {
    const truthCount = list(item.truth_sources).length;
    parts.push(truthCount > 1 ? t("counts.truth", { count: truthCount }) : t("registry.truth"));
  }
  if (item.kind === "unit" && list(item.implementation_paths).length > 0) {
    parts.push(t("counts.paths", { count: list(item.implementation_paths).length }));
  }
  if (item.mapping_registered) parts.push(t("registry.mapping"));
  return parts.length > 0 ? parts.join(" · ") : t("fallback.none");
}

function registryTruthSummary(item) {
  const count = list(item.truth_sources).length;
  if (count === 0) return t("registry.no");
  if (count === 1) return list(item.truth_sources)[0].path || t("registry.truth");
  return t("counts.truth", { count });
}

function registryImplementationPathSummary(item) {
  const paths = list(item.implementation_paths).map((ref) => ref.path).filter(Boolean);
  if (paths.length === 0) return t("fallback.none");
  if (paths.length === 1) return paths[0];
  return t("counts.paths", { count: paths.length });
}

function renderRegistryImplementationPaths(item) {
  const refs = list(item.implementation_paths).filter((ref) => ref && ref.path);
  if (refs.length === 0) return `<span class="registry-path-empty">${escapeHTML(t("fallback.none"))}</span>`;
  return `
    <div class="registry-path-list">
      ${refs.map((ref) => {
        const source = sourceForImplementationRef(ref);
        if (source && isReadableOriginalPath(source.path)) {
          return `
            <button class="registry-path" type="button" data-source="${escapeAttr(source.path)}" ${source.line ? `data-source-line="${escapeAttr(source.line)}"` : ""}>
              ${escapeHTML(ref.path)}
            </button>
          `;
        }
        return `<span class="registry-path">${escapeHTML(ref.path)}</span>`;
      }).join("")}
    </div>
  `;
}

function registryIssueSummary(item) {
  const issues = list(item.issues);
  return issues.length > 0 ? issues.join("; ") : t("registry.noIssues");
}

function registryAttentionSummary(item) {
  const issues = list(item.issues);
  if (issues.length > 0) return issues.join("; ");
  if (item.result === "planned") return t("registry.planned");
  return t("registry.landed");
}

function bindRegistryBoardLinks() {
  graphView.querySelectorAll("[data-source]").forEach((button) => {
    button.addEventListener("click", (event) => {
      event.preventDefault();
      const line = Number(button.dataset.sourceLine || 0);
      openSource(button.dataset.source, line > 0 ? { line } : {});
    });
  });
  graphView.querySelectorAll("[data-registry]").forEach((button) => {
    button.addEventListener("click", () => focusNode(button.dataset.registry));
  });
}

function ruleSourcesForObject(object) {
  return list(object.rule_refs).flatMap((ruleID) => {
    const rule = list(snapshot.objects).find((item) => item.kind === "rule" && item.id === ruleID);
    return rule ? uniqueSources(rule.truth_paths) : [];
  });
}

function sourceExists(path) {
  if (!isReadableOriginalPath(path)) return false;
  return list(snapshot.sources).some((source) => source.path === path)
    || list(snapshot.nodes).some((node) => node.source && node.source.path === path)
    || list(snapshot.objects).some((object) => list(object.truth_paths).some((ref) => ref.path === path));
}

function isAppendixPath(path) {
  return String(path || "").includes("/appendix/");
}

function renderReviewBoard() {
  graphView.innerHTML = "";
}

function reviewObjectGroups(items) {
  const groups = new Map();
  list(items).forEach((item) => {
    if (!item || !item.object) return;
    const key = reviewObjectGroupKey(item);
    if (!groups.has(key)) {
      groups.set(key, {
        key,
        reviewType: item.reviewType,
        object: item.object,
        objectLabel: item.objectLabel || item.object.label || item.object.id || t("fallback.undeclared"),
        items: []
      });
    }
    groups.get(key).items.push(item);
  });
  const result = [...groups.values()];
  result.forEach((group) => {
    group.items.sort(compareReviewItems);
  });
  result.sort(compareReviewGroups);
  return result;
}

function reviewObjectGroupKey(item) {
  if (!item || !item.object) return "";
  return `review-object:${item.reviewType}:${item.object.kind}:${item.object.id}`;
}

function compareReviewGroups(left, right) {
  return reviewTypeOrder().indexOf(left.reviewType) - reviewTypeOrder().indexOf(right.reviewType)
    || String(left.objectLabel || "").localeCompare(String(right.objectLabel || ""))
    || String(left.key || "").localeCompare(String(right.key || ""));
}

function ensureReviewSectionExpansion(type, groups) {
  if (initializedReviewNavGroups.has(type)) return;
  const selectedGroup = groups.find((group) => group.items.some((item) => item.id === selectedNodeID));
  const defaultGroup = selectedGroup || groups[0];
  if (defaultGroup) expandedReviewObjectKeys.add(defaultGroup.key);
  initializedReviewNavGroups.add(type);
}

function focusReviewItem(itemID) {
  const item = reviewItemByID(itemID);
  if (item) {
    activeReviewNavGroup = item.reviewType;
    initializedReviewNavGroups.add(item.reviewType);
    expandedReviewObjectKeys.add(reviewObjectGroupKey(item));
  }
  selectedNodeID = itemID;
  renderNav();
  renderGraph();
  renderDetailForNode(itemID);
}

function reviewItems() {
  const items = [];
  const seen = new Set();
  const addItem = (item) => {
    if (!item || !item.path || !isReadableOriginalPath(item.path)) return;
    const key = `${item.reviewType}:${item.path}:${item.object ? item.object.id : item.objectLabel}`;
    if (seen.has(key)) return;
    seen.add(key);
    const source = item.source || { path: item.path };
    items.push({
      ...item,
      id: `spec:${item.reviewType}:${item.path}:${item.object ? item.object.id : item.objectLabel}`,
      fileLabel: reviewFileLabel(source, item.object),
      source,
      stateLabel: item.stateLabel || t(`review.states.${item.reviewType}`),
      targetType: item.targetType || reviewTargetTypeForObject(item.object)
    });
  };

  list(snapshot.objects).forEach((object) => {
    const targetType = reviewTargetTypeForObject(object);
    if (!targetType) return;
    if (object.kind === "unit" && object.layer === "candidate") {
      uniqueSources(object.truth_paths).filter((source) => isReviewSourceForLayer(source, object, "candidate")).forEach((source) => {
        addItem({
          reviewType: "candidate",
          targetType,
          path: source.path,
          source,
          object,
          objectLabel: object.label || object.id || t("fallback.undeclared")
        });
      });
      return;
    }
    if (object.kind === "unit" && object.layer === "stable") {
      uniqueSources(object.truth_paths).filter((source) => isReviewSourceForLayer(source, object, "stable")).forEach((source) => {
        addItem({
          reviewType: "stable",
          targetType,
          path: source.path,
          source,
          object,
          objectLabel: object.label || object.id || t("fallback.undeclared")
        });
      });
      return;
    }
    if (object.kind !== "rule" || object.layer !== "stable") return;
    uniqueSources(object.truth_paths).filter((source) => isReviewSourceForLayer(source, object, "stable")).forEach((source) => {
      addItem({
        reviewType: "stableRule",
        targetType,
        path: source.path,
        source,
        object,
        objectLabel: object.label || object.id || t("fallback.undeclared")
      });
    });
  });

  return items.sort(compareReviewItems);
}

function reviewItemByID(itemID) {
  return reviewItems().find((item) => item.id === itemID) || null;
}

function reviewItemByPath(path) {
  if (currentView !== "spec") return null;
  return reviewItems().find((item) => item.path === path) || null;
}

function reviewTargetTypeForObject(object) {
  if (!object) return "";
  if (object.kind === "unit") return "capability";
  if (object.kind === "rule") return "rule";
  return "";
}

function reviewTypeOrder() {
  return ["candidate", "stable", "stableRule"];
}

function compareReviewItems(left, right) {
  return reviewTypeOrder().indexOf(left.reviewType) - reviewTypeOrder().indexOf(right.reviewType)
    || String(left.objectLabel || "").localeCompare(String(right.objectLabel || ""))
    || reviewDocKindRank(left) - reviewDocKindRank(right)
    || String(left.path || "").localeCompare(String(right.path || ""));
}

function reviewDocKindRank(item) {
  const kind = reviewDocKind(item && item.source ? item.source : item);
  if (kind === "main") return 0;
  if (kind === "appendix") return 1;
  if (kind === "evidence") return 2;
  return 3;
}

function isReviewSourceForLayer(source, object, layer) {
  const path = String(source && source.path ? source.path : "");
  if (!path.includes(`/${layer}/`)) return false;
  if (object.kind === "unit") return isUnitReviewSource(source, object, layer);
  if (object.kind === "rule") return isRuleReviewSource(source, object, layer);
  return false;
}

function isUnitReviewSource(source, object, layer) {
  const path = String(source && source.path ? source.path : "");
  const name = fileName(path);
  const prefix = layer === "stable" ? "s" : "c";
  const objectID = normalizedObjectID(object);
  if (!objectID) return false;
  if (isAppendixPath(path)) return name.startsWith(`${prefix}_unit_${objectID}_`) && name.endsWith(".md");
  return name === `${prefix}_unit_${objectID}.md`;
}

function isRuleReviewSource(source, object, layer) {
  const path = String(source && source.path ? source.path : "");
  if (isAppendixPath(path)) return false;
  const prefix = layer === "stable" ? "s" : "c";
  const objectID = normalizedObjectID(object);
  if (!objectID) return false;
  return fileName(path) === `${prefix}_${objectID}.md`;
}

function isPrimaryReviewSource(source, object, layer) {
  const path = String(source && source.path ? source.path : "");
  return isReviewSourceForLayer(source, object, layer) && !isAppendixPath(path);
}

function isAppendixReference(source) {
  const path = String(source && source.path ? source.path : "");
  return path.includes("/appendix/") && !isEvidenceReference(source);
}

function isEvidenceReference(source) {
  const path = String(source && source.path ? source.path : "");
  const label = String(source && source.label ? source.label : "").toLowerCase();
  return path.includes("/appendix/") && (label === "evidence appendix" || label === "evidence");
}

function isStableReference(source) {
  const path = String(source && source.path ? source.path : "");
  return path.includes("/stable/") || fileName(path).startsWith("s_");
}

function reviewTypeLabel(type) {
  return t(`review.types.${type}`);
}

function reviewTarget(type) {
  return t(`review.targets.${type}`);
}

function reviewFocusItems(type) {
  return String(t(`review.focus.${type}`))
    .split(/[、,]/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function reviewNavSubtitle(item) {
  const state = item.stateLabel || reviewTypeLabel(item.reviewType);
  return [state, reviewDocKindLabel(item), item.objectLabel].filter(Boolean).join(" · ");
}

function reviewTreeFileLabel(item) {
  const kind = reviewDocKind(item && item.source ? item.source : item);
  if (kind === "main") return t("review.docKinds.main");
  const label = item && item.fileLabel ? item.fileLabel : reviewFileLabel(item && item.source, item && item.object);
  if (kind === "evidence") return `${t("review.docKinds.evidenceAppendix")} ${label}`;
  return `${t("review.docKinds.appendix")} ${label}`;
}

function reviewTreeFileSubtitle(item) {
  const state = item && item.stateLabel ? item.stateLabel : reviewTypeLabel(item.reviewType);
  return [state, item && item.path].filter(Boolean).join(" · ");
}

function reviewDocKindLabel(item) {
  return t(`review.docKinds.${reviewDocKind(item && item.source ? item.source : item)}`);
}

function reviewDocKind(source) {
  if (isEvidenceReference(source)) return "evidence";
  if (isAppendixReference(source)) return "appendix";
  return "main";
}

function reviewFileLabel(source, object) {
  const namePart = reviewFileNamePart(source, object);
  if (namePart) return namePart;
  return compactTruthFileLabel(fileName(source && source.path));
}

function reviewFileNamePart(source, object) {
  const stem = reviewFileStem(source);
  if (!stem) return "";
  const prefix = reviewFilePrefix(source, object);
  if (!prefix) return stem;
  if (stem === prefix) return normalizedObjectID(object);
  if (stem.startsWith(`${prefix}_`)) return stem.slice(prefix.length + 1);
  return stem;
}

function reviewFileStem(source) {
  const path = String(source && source.path ? source.path : "");
  return fileName(path).replace(/\.md$/, "");
}

function reviewFilePrefix(source, object) {
  const objectID = normalizedObjectID(object);
  if (!objectID) return "";
  const path = String(source && source.path ? source.path : "");
  const stem = reviewFileStem(source);
  const layerPrefix = path.includes("/stable/") || stem.startsWith("s_") ? "s" : "c";
  if (object && object.kind === "unit") return `${layerPrefix}_unit_${objectID}`;
  if (object && object.kind === "rule") return `${layerPrefix}_${objectID}`;
  return `${layerPrefix}_${objectID}`;
}

function normalizedObjectID(object) {
  return String(object && object.id ? object.id : "").replace(/-/g, "_");
}

function reviewRelationSummary(item) {
  const parts = reviewRelationGroups(item)
    .map((group) => `${group.label} ${group.items.length}`);
  return parts.length > 0 ? parts.join(" · ") : t("review.relationEmpty");
}

function reviewRelationGroups(item) {
  const object = item ? item.object : null;
  if (!object) return [];
  const groups = [];
  const implementation = list(object.implementation_paths).map((ref) => ref.path).filter(Boolean);
  if (implementation.length > 0) groups.push({ label: t("review.relation.implementation"), items: implementation, linkable: false });
  const ruleFiles = ruleSourcesForObject(object).map((ref) => ref.path).filter(Boolean);
  if (ruleFiles.length > 0) {
    groups.push({ label: t("review.relation.ruleFile"), items: ruleFiles, linkable: true });
  } else {
    const rules = list(object.rule_refs).filter(Boolean);
    if (rules.length > 0) groups.push({ label: t("review.relation.rule"), items: rules, linkable: false });
  }
  const bound = list(object.bound_objects).filter(Boolean);
  if (bound.length > 0) groups.push({ label: t("review.relation.bound"), items: bound, linkable: false });
  const appendix = uniqueSources(object.truth_paths)
    .filter((ref) => isAppendixReference(ref))
    .map((ref) => ref.path);
  if (appendix.length > 0) groups.push({ label: t("review.relation.appendix"), items: appendix, linkable: true });
  const evidence = uniqueSources(object.truth_paths)
    .filter((ref) => isEvidenceReference(ref))
    .map((ref) => ref.path);
  if (evidence.length > 0) groups.push({ label: t("review.relation.evidence"), items: evidence, linkable: true });
  const stable = uniqueSources(object.truth_paths)
    .filter((ref) => isStableReference(ref))
    .map((ref) => ref.path);
  if (stable.length > 0) groups.push({ label: t("review.relation.stable"), items: stable, linkable: true });
  if (snapshot.project.mapping_file) {
    groups.push({ label: t("review.relation.mapping"), items: [snapshot.project.mapping_file], linkable: true });
  }
  if (snapshot.project.rule_baseline_file) {
    groups.push({ label: t("review.relation.system"), items: [snapshot.project.rule_baseline_file], linkable: true });
  }
  return groups;
}

function renderReviewDetail(item) {
  if (!item) {
    detailPanel.innerHTML = `<h2>${escapeHTML(t("fallback.noObject"))}</h2>`;
    updateTruthTab([]);
    return;
  }
  detailPanel.innerHTML = `
    <h2>${escapeHTML(item.fileLabel)}</h2>
    <dl class="detail-grid">
      <dt>${escapeHTML(t("review.fileType"))}</dt><dd>${escapeHTML(reviewTypeLabel(item.reviewType))}</dd>
      <dt>${escapeHTML(t("review.object"))}</dt><dd class="detail-kind">${renderKindBadge(item.object.kind)}<span>${escapeHTML(item.objectLabel)}</span></dd>
      <dt>${escapeHTML(t("inspector.fields.status"))}</dt><dd>${escapeHTML(item.stateLabel || reviewTypeLabel(item.reviewType))}</dd>
      <dt>${escapeHTML(t("inspector.fields.file"))}</dt><dd>${escapeHTML(item.path)}</dd>
    </dl>
    <section class="review-detail-section">
      <h2>${escapeHTML(t("review.reviewTarget"))}</h2>
      <p>${escapeHTML(reviewTarget(item.reviewType))}</p>
    </section>
    <section class="review-detail-section">
      <h2>${escapeHTML(t("review.readingFocus"))}</h2>
      <ul class="review-focus-points">
        ${reviewFocusItems(item.reviewType).map((focus) => `<li>${escapeHTML(focus)}</li>`).join("")}
      </ul>
    </section>
    <section class="review-detail-section">
      <h2>${escapeHTML(t("review.relationships"))}</h2>
      ${renderReviewRelationGroups(item)}
    </section>
    <section class="review-detail-section">
      ${renderSourceButton(item.path, t("review.openSource"))}
    </section>
  `;
  bindInspectorLinks();
  updateTruthTab([item.source], item.id, { activate: true });
}

function renderReviewEmptyDetail() {
  detailPanel.innerHTML = `
    <section class="review-empty-state">
      <h2>${escapeHTML(t("review.emptyDetailTitle"))}</h2>
      <p>${escapeHTML(t("review.emptyDetail"))}</p>
    </section>
  `;
  updateTruthTab([], "review-empty");
}

function renderReviewRelationGroups(item) {
  const groups = reviewRelationGroups(item);
  if (groups.length === 0) return `<p class="empty-copy">${escapeHTML(t("review.relationEmpty"))}</p>`;
  return groups.map((group) => {
    const chips = group.items.map((value) => {
      if (group.linkable) {
        return `<button class="chip" type="button" data-source="${escapeAttr(value)}">${escapeHTML(value)}</button>`;
      }
      return `<span class="chip">${escapeHTML(value)}</span>`;
    }).join("");
    return `<h3 class="review-relation-title">${escapeHTML(group.label)}</h3><div class="chips">${chips}</div>`;
  }).join("");
}

function fileName(path) {
  return String(path || "").split("/").pop() || String(path || "");
}

function compactLabel(node) {
  const label = String(node.label || "");
  if (node.kind === "project_path" || node.kind === "project_area") {
    return label
      .replace(/^docs\/specs\/units\/candidate\/appendix\//, "appendix/")
      .replace(/^docs\/specs\/units\/candidate\//, "units/candidate/")
      .replace(/^docs\/specs\/units\/stable\//, "units/stable/")
      .replace(/^docs\/specs\/rules\/candidate\//, "rule/candidate/")
      .replace(/^docs\/specs\/rules\/stable\//, "rule/stable/")
      .replace("/**", "\n/**");
  }
  if (node.kind === "truth_file") {
    return compactTruthFileLabel(label);
  }
  if (node.kind === "implementation_path") {
    return label.replace("/**", "\n/**");
  }
  return label;
}

function compactTruthFileLabel(label) {
  const base = String(label || "").replace(/\.md$/, "");
  const unitMatch = base.match(/^([cs])_unit_(.+)$/);
  if (unitMatch) return `${unitMatch[2].replace(/_/g, " ")} (${truthLayerName(unitMatch[1])})`;
  const sharedMatch = base.match(/^([cs])_[gb]_rule_(.+)$/);
  if (sharedMatch) return `rule ${sharedMatch[2].replace(/_/g, " ")} (${truthLayerName(sharedMatch[1])})`;
  return base.replace(/_/g, " ");
}

function truthLayerName(prefix) {
  return prefix === "s" ? "stable" : "candidate";
}

function edgeLabel(kind) {
  if (kind === "described_by") return "Spec";
  if (kind === "owns_path") return "Path";
  if (kind === "uses_rule") return "Uses";
  if (kind === "bound_to") return "Bound";
  if (kind === "contains") return "Contains";
  if (kind === "maps_to") return "Owner";
  if (kind === "declares") return "Declares";
  if (kind === "tracks_state") return "State";
  if (kind === "constrains") return "Constrains";
  return kind;
}

function nodeSize(ele) {
  const group = ele.data("group");
  if (group === "root") return 48;
  if (group === "unit") return 42;
  if (group === "rule") return 40;
  if (group === "truth") return 34;
  return 36;
}

function edgeWidth(ele) {
  const kind = ele.data("kind");
  if (kind === "uses_rule" || kind === "bound_to" || kind === "maps_to") return 2;
  return 1.5;
}

function average(values) {
  return values.reduce((sum, value) => sum + value, 0) / values.length;
}

function byLabel(left, right) {
  return String(left.label || "").localeCompare(String(right.label || ""));
}

function renderDetail(object) {
  if (!object) {
    detailPanel.innerHTML = `<h2>${escapeHTML(t("fallback.noObject"))}</h2>`;
    updateTruthTab([]);
    return;
  }
  const truthRefs = truthRefsForObject(object);
  detailPanel.innerHTML = `
    <h2>${escapeHTML(object.label)}</h2>
    <dl class="detail-grid">
      <dt>${escapeHTML(t("inspector.fields.type"))}</dt><dd class="detail-kind">${renderKindBadge(object.kind)}<span>${escapeHTML(object.kind)}</span></dd>
      <dt>${escapeHTML(t("inspector.fields.status"))}</dt><dd>${escapeHTML(object.layer || t("fallback.undeclared"))}</dd>
      <dt>${escapeHTML(t("inspector.fields.version"))}</dt><dd>${escapeHTML(object.version || t("fallback.undeclared"))}</dd>
      <dt>${escapeHTML(t("inspector.fields.responsibility"))}</dt><dd>${escapeHTML(object.responsibility || t("fallback.undeclared"))}</dd>
    </dl>
    ${renderChipGroup(t("inspector.groups.truth"), object.truth_paths, true)}
    ${renderImplementationPathGroup(t("inspector.groups.implementation"), object.implementation_paths)}
    ${renderTextChips(t("inspector.groups.rule"), object.rule_refs)}
    ${renderTextChips(t("inspector.groups.bound"), object.bound_objects)}
  `;
  bindInspectorLinks();
  updateTruthTab(truthRefs, objectNodeID(object));
}

function renderDetailForNode(nodeID) {
  if (currentView === "spec") {
    const item = reviewItemByID(nodeID);
    if (item) {
      renderReviewDetail(item);
      return;
    }
    renderReviewEmptyDetail();
    return;
  }
  if (currentView === "registry") {
    const item = registryItemByID(nodeID);
    if (item) {
      renderRegistryDetail(item);
      return;
    }
    renderRegistryEmptyDetail();
    return;
  }
  const object = objectFromNode(nodeID);
  if (object) {
    renderDetail(object);
    return;
  }
  const graph = graphForCurrentView();
  const node = graph.nodes.find((item) => item.id === nodeID);
  if (!node) {
    detailPanel.innerHTML = `<h2>${escapeHTML(t("fallback.noObject"))}</h2>`;
    updateTruthTab([], nodeID);
    return;
  }
  if (currentView === "project" && node.kind === "project_area") {
    renderProjectAreaDetail(node, graph);
    return;
  }
  if (currentView === "project" && node.kind === "project_root") {
    renderProjectRootDetail(node, graph);
    return;
  }
  const truthRefs = truthRefsForNode(node);
  const outgoing = graph.edges.filter((edge) => edge.from === nodeID);
  const incoming = graph.edges.filter((edge) => edge.to === nodeID);
  const connected = outgoing.concat(incoming).map((edge) => edge.from === nodeID ? edge.to : edge.from);
  const connectedNodes = connected
    .map((id) => graph.nodes.find((item) => item.id === id))
    .filter(Boolean);
  detailPanel.innerHTML = `
    <h2>${escapeHTML(node.label)}</h2>
    <dl class="detail-grid">
      <dt>${escapeHTML(t("inspector.fields.type"))}</dt><dd>${escapeHTML(labelForKind(node.kind))}</dd>
      ${node.source && node.source.path ? `<dt>${escapeHTML(t("inspector.fields.file"))}</dt><dd>${escapeHTML(node.source.path)}</dd>` : ""}
      <dt>${escapeHTML(t("inspector.fields.connections"))}</dt><dd>${incoming.length + outgoing.length}</dd>
    </dl>
    ${renderChipGroup(t("inspector.groups.truth"), truthRefs, true)}
    ${renderNodeList(t("inspector.groups.connected"), connectedNodes)}
  `;
  bindInspectorLinks();
  updateTruthTab(truthRefs, nodeID);
}

function renderProjectAreaDetail(node, graph) {
  const ownerNodes = graph.edges
    .filter((edge) => edge.from === node.id && edge.kind === "maps_to")
    .map((edge) => graph.nodes.find((item) => item.id === edge.to))
    .filter(Boolean)
    .sort(byLabel);
  const truthRefs = uniqueSources(list(node.raw_paths).map(sourceForImplementationRef));
  detailPanel.innerHTML = `
    <h2>${escapeHTML(node.aggregate_path_label || node.label)}</h2>
    <dl class="detail-grid">
      <dt>${escapeHTML(t("inspector.fields.type"))}</dt><dd>${escapeHTML(labelForKind(node.kind))}</dd>
      <dt>${escapeHTML(t("inspector.fields.connections"))}</dt><dd>${escapeHTML(ownerNodes.length)}</dd>
      <dt>${escapeHTML(t("inspector.fields.paths"))}</dt><dd>${escapeHTML(t("counts.paths", { count: list(node.raw_paths).length }))}</dd>
    </dl>
    ${renderImplementationPathGroup(t("inspector.groups.implementation"), node.raw_paths)}
    ${renderNodeList(t("inspector.groups.connected"), ownerNodes)}
  `;
  bindInspectorLinks();
  updateTruthTab(truthRefs, node.id);
}

function renderProjectRootDetail(node, graph) {
  const areaNodes = graph.edges
    .filter((edge) => edge.from === node.id && edge.kind === "contains")
    .map((edge) => graph.nodes.find((item) => item.id === edge.to))
    .filter(Boolean)
    .sort(byLabel);
  const truthRefs = uniqueSources(list(node.raw_paths).map(sourceForImplementationRef));
  detailPanel.innerHTML = `
    <h2>${escapeHTML(node.label)}</h2>
    <dl class="detail-grid">
      <dt>${escapeHTML(t("inspector.fields.type"))}</dt><dd>${escapeHTML(labelForKind(node.kind))}</dd>
      <dt>${escapeHTML(t("inspector.fields.connections"))}</dt><dd>${escapeHTML(areaNodes.length)}</dd>
      <dt>${escapeHTML(t("inspector.fields.paths"))}</dt><dd>${escapeHTML(t("counts.paths", { count: list(node.raw_paths).length }))}</dd>
    </dl>
    ${renderNodeList(t("views.project.groups.areas"), areaNodes)}
  `;
  bindInspectorLinks();
  updateTruthTab(truthRefs, node.id);
}

function renderRegistryDetail(item) {
  const truthRefs = uniqueSources(list(item.truth_sources).concat(list(item.sources)));
  detailPanel.innerHTML = `
    <h2>${escapeHTML(item.label || item.id)}</h2>
    <dl class="detail-grid">
      <dt>${escapeHTML(t("inspector.fields.type"))}</dt><dd class="detail-kind">${renderRegistryKindBadge(item)}<span>${escapeHTML(registryKindText(item))}</span></dd>
      <dt>${escapeHTML(t("registry.result"))}</dt><dd>${renderRegistryResult(item.result)}</dd>
      <dt>${escapeHTML(t("registry.mapping"))}</dt><dd>${renderRegistryMappingPresence(item)}</dd>
      <dt>${escapeHTML(t("registry.status"))}</dt><dd>${renderRegistryStatusPresence(item)}</dd>
      <dt>${escapeHTML(t("registry.truth"))}</dt><dd>${renderRegistryPresence(item.truth_registered, firstSourceRef(item.truth_sources))}</dd>
      <dt>${escapeHTML(t("registry.implementation"))}</dt><dd>${escapeHTML(registryImplementationSummary(item))}</dd>
    </dl>
    <section class="review-detail-section">
      <h2>${escapeHTML(t("registry.sourceChain"))}</h2>
      ${renderRegistrySourceChain(item)}
    </section>
    ${renderTextChips(t("registry.unitRefs"), item.unit_refs)}
    ${renderTextChips(t("registry.ruleRefs"), item.rule_refs)}
    ${renderTextChips(t("registry.boundObjects"), item.bound_objects)}
    ${renderImplementationPathGroup(t("registry.implementation"), item.implementation_paths)}
    ${renderRegistryIssues(item)}
  `;
  bindInspectorLinks();
  updateTruthTab(truthRefs, registryNodeID(item));
}

function renderRegistrySourceChain(item) {
  const groups = [];
  if (item.mapping_source && item.mapping_source.path) {
    groups.push({ label: t("registry.mappingSource"), refs: [item.mapping_source] });
  }
  if (item.status_source && item.status_source.path) {
    groups.push({ label: t("registry.statusSource"), refs: [item.status_source] });
  }
  if (list(item.truth_sources).length > 0) {
    groups.push({ label: t("registry.truthSources"), refs: item.truth_sources });
  }
  if (groups.length === 0) return `<p class="empty-copy">${escapeHTML(t("fallback.none"))}</p>`;
  return groups.map((group) => `
    <h3 class="review-relation-title">${escapeHTML(group.label)}</h3>
    <div class="chips">
      ${list(group.refs).map((ref) => `<button class="chip" type="button" data-source="${escapeAttr(ref.path)}" ${ref.line ? `data-source-line="${escapeAttr(ref.line)}"` : ""}>${escapeHTML(ref.path)}${ref.line ? `:${escapeHTML(ref.line)}` : ""}</button>`).join("")}
    </div>
  `).join("");
}

function renderRegistryIssues(item) {
  const issues = list(item.issues);
  if (issues.length === 0) {
    return `<h2>${escapeHTML(t("registry.issues"))}</h2><p class="empty-copy">${escapeHTML(t("registry.noIssues"))}</p>`;
  }
  return `
    <h2>${escapeHTML(t("registry.issues"))}</h2>
    <ul class="review-focus-points">
      ${issues.map((issue) => `<li>${escapeHTML(issue)}</li>`).join("")}
    </ul>
  `;
}

function renderRegistryEmptyDetail() {
  detailPanel.innerHTML = `<h2>${escapeHTML(t("fallback.noObject"))}</h2>`;
  updateTruthTab([], "registry-empty");
}

function bindInspectorLinks() {
  detailPanel.querySelectorAll("[data-source]").forEach((link) => {
    link.addEventListener("click", (event) => {
      event.preventDefault();
      const line = Number(link.dataset.sourceLine || 0);
      openSource(link.dataset.source, line > 0 ? { line } : {});
    });
  });
  detailPanel.querySelectorAll("[data-node]").forEach((button) => {
    button.addEventListener("click", () => focusNode(button.dataset.node));
  });
}

function truthRefsForObject(object) {
  return uniqueSources((object.truth_paths || []).concat(object.sources || []));
}

function truthRefsForNode(node) {
  if (node && list(node.raw_paths).length > 0) {
    return uniqueSources(list(node.raw_paths).map(sourceForImplementationRef));
  }
  if (!node || !node.source || !isReadableOriginalPath(node.source.path)) return [];
  return [node.source];
}

function uniqueSources(sources) {
  const seen = new Set();
  return list(sources).filter(Boolean).filter((source) => {
    if (!source.path || seen.has(source.path)) return false;
    seen.add(source.path);
    return true;
  });
}

function updateTruthTab(truthRefs, ownerID, options = {}) {
  const refs = uniqueSources(truthRefs).filter((ref) => isReadableOriginalPath(ref.path));
  const hasTruth = refs.length > 0;
  const activateTruth = options.activate === true;
  truthTab.classList.toggle("hidden", !hasTruth);
  if (!hasTruth) {
    activeTruthOwnerID = null;
    sourcePath.textContent = "";
    sourceContent.textContent = t("source.emptyRaw");
    sourceRendered.textContent = t("source.emptyRendered");
    activeSourceDiff = null;
    expandedDiffMarkers = new Set();
    activeSourceHeadings = [];
    renderDocGuide([]);
    updateDiffToggle();
    setInspectorTab("info");
    return;
  }
  if (activeTruthOwnerID !== ownerID || !refs.some((ref) => ref.path === sourcePath.textContent)) {
    activeTruthOwnerID = ownerID;
    openSource(refs[0].path, { activate: activateTruth });
  }
  setInspectorTab(activateTruth || activeInspectorTab === "truth" ? "truth" : "info");
}

function setInspectorTab(tabName) {
  if (tabName === "truth" && truthTab.classList.contains("hidden")) {
    tabName = "info";
  }
  activeInspectorTab = tabName;
  infoTab.classList.toggle("active", tabName === "info");
  truthTab.classList.toggle("active", tabName === "truth");
  detailPanel.classList.toggle("hidden", tabName !== "info");
  truthPanel.classList.toggle("hidden", tabName !== "truth");
}

function setDocMode(mode) {
  activeDocMode = mode === "raw" ? "raw" : "rendered";
  document.querySelectorAll("[data-doc-mode]").forEach((button) => {
    button.classList.toggle("active", button.dataset.docMode === activeDocMode);
  });
  sourceRendered.classList.toggle("hidden", activeDocMode !== "rendered");
  sourceContent.classList.toggle("hidden", activeDocMode !== "raw");
}

function isReadableOriginalPath(path) {
  if (!path) return false;
  return path.startsWith("docs/specs/")
    || path === "AGENTS.md"
    || path === "CLAUDE.md"
    || path === "GEMINI.md";
}

function startInspectorResize(event) {
  event.preventDefault();
  resizeBar.classList.add("dragging");
  const minWidth = 220;
  const onPointerMove = (moveEvent) => {
    const nextWidth = Math.round(window.innerWidth - moveEvent.clientX - 12);
    const clampedWidth = Math.max(minWidth, nextWidth);
    document.documentElement.style.setProperty("--inspector-width", `${clampedWidth}px`);
    if (cy) cy.resize();
  };
  const stopResize = () => {
    resizeBar.classList.remove("dragging");
    window.removeEventListener("pointermove", onPointerMove);
    window.removeEventListener("pointerup", stopResize);
    if (cy) cy.resize();
  };
  window.addEventListener("pointermove", onPointerMove);
  window.addEventListener("pointerup", stopResize);
}

function renderNodeList(title, nodes) {
  if (!nodes || nodes.length === 0) return "";
  return `<h2>${title}</h2><div class="chips">${nodes.map((node) => `<button class="chip" type="button" data-node="${escapeAttr(node.id)}">${escapeHTML(node.label)}</button>`).join("")}</div>`;
}

function labelForKind(kind) {
  const translated = lookupTranslation(TRANSLATIONS[currentLanguage], `kind.${kind}`)
    ?? lookupTranslation(TRANSLATIONS["zh-CN"], `kind.${kind}`);
  if (translated) return translated;
  return kind;
}

function objectKindLabel(kind) {
  const translated = lookupTranslation(TRANSLATIONS[currentLanguage], `legend.${kind}.label`)
    ?? lookupTranslation(TRANSLATIONS["zh-CN"], `legend.${kind}.label`);
  if (translated) return translated;
  return labelForKind(kind);
}

function objectKindClass(kind) {
  const normalized = String(kind || "").trim();
  if (normalized === "unit" || normalized === "rule") return `kind-${normalized}`;
  return "";
}

function renderKindBadge(kind) {
  const className = objectKindClass(kind);
  if (!className) return "";
  return `<span class="object-kind-badge ${escapeAttr(className)}">${escapeHTML(objectKindLabel(kind))}</span>`;
}

function registryKindText(item) {
  if (item.kind === "rule") {
    if (item.rule_scope === "global") return t("registry.ruleScope.global");
    if (item.rule_scope === "bound") return t("registry.ruleScope.bound");
    return t("registry.ruleScope.unknown");
  }
  return objectKindLabel(item.kind);
}

function renderRegistryKindBadge(item) {
  const className = objectKindClass(item.kind);
  if (!className) return "";
  return `<span class="object-kind-badge ${escapeAttr(className)}">${escapeHTML(registryKindText(item))}</span>`;
}

function renderNavItemTitle(label, kind) {
  return `
    <span class="nav-item-title">
      ${renderKindBadge(kind)}
      <strong>${escapeHTML(label)}</strong>
    </span>
  `;
}

function renderImplementationPathGroup(title, refs) {
  if (!refs || refs.length === 0) return "";
  const chips = refs.map((ref) => {
    const source = sourceForImplementationRef(ref);
    if (source && isReadableOriginalPath(source.path)) {
      return `<button class="chip" type="button" data-source="${escapeAttr(source.path)}" ${source.line ? `data-source-line="${escapeAttr(source.line)}"` : ""}>${escapeHTML(ref.path)}</button>`;
    }
    return `<span class="chip">${escapeHTML(ref.path)}</span>`;
  }).join("");
  return `<h2>${title}</h2><div class="chips">${chips}</div>`;
}

function sourceForImplementationRef(ref) {
  if (!ref) return null;
  const sourcePath = ref.label && isReadableOriginalPath(ref.label) ? ref.label : snapshot.project.mapping_file;
  if (!sourcePath) return null;
  return { path: sourcePath, line: ref.line || 0, label: ref.path || sourcePath };
}

function renderChipGroup(title, refs, linkable) {
  if (!refs || refs.length === 0) return "";
  const chips = refs.map((ref) => {
    if (linkable) {
      return `<button class="chip" type="button" data-source="${escapeAttr(ref.path)}">${escapeHTML(ref.path)}</button>`;
    }
    return `<span class="chip">${escapeHTML(ref.path)}</span>`;
  }).join("");
  return `<h2>${title}</h2><div class="chips">${chips}</div>`;
}

function renderTextChips(title, items) {
  if (!items || items.length === 0) return "";
  return `<h2>${title}</h2><div class="chips">${items.map((item) => `<span class="chip">${escapeHTML(item)}</span>`).join("")}</div>`;
}

function renderSources(sources) {
  const seen = new Set();
  return list(sources).filter(Boolean).filter((source) => {
    if (!source.path || seen.has(source.path)) return false;
    seen.add(source.path);
    return true;
  }).map((source) => `<a href="#" class="source-link" data-source="${escapeAttr(source.path)}">${escapeHTML(source.path)}${source.line ? ":" + source.line : ""}</a>`).join("");
}

async function openSource(path, options = {}) {
  const activate = options.activate !== false;
  const targetLine = Number(options.line || 0);
  const response = await fetch(`/api/source?path=${encodeURIComponent(path)}`);
  if (!response.ok) {
    const message = await response.text();
    sourcePath.textContent = path;
    sourceContent.textContent = message;
    sourceRendered.textContent = message;
    activeSourceDiff = null;
    expandedDiffMarkers = new Set();
    activeSourceHeadings = [];
    renderDocGuide([]);
    updateDiffToggle();
    setDocMode(activeDocMode);
    if (activate) setInspectorTab("truth");
    return;
  }
  const source = await response.json();
  const renderedDoc = renderMarkdownDocument(source.content);
  activeSourceDiff = await fetchSourceDiff(source.path);
  diffMarkersEnabled = Boolean(activeSourceDiff && activeSourceDiff.available);
  expandedDiffMarkers = new Set();
  sourcePath.textContent = source.path;
  sourceContent.textContent = source.content;
  sourceRendered.innerHTML = renderedDoc.html;
  activeSourceHeadings = renderedDoc.headings;
  applyDiffAnnotations();
  renderDocGuide(activeSourceHeadings);
  updateDiffToggle();
  bindRenderedDocLinks(source.path);
  bindDocGuideLinks();
  bindDiffMarkers();
  renderMermaidBlocks();
  if (targetLine > 0) {
    setDocMode("raw");
    requestAnimationFrame(() => scrollRawSourceToLine(targetLine));
  } else {
    setDocMode(activeDocMode);
  }
  if (activate) setInspectorTab("truth");
}

async function fetchSourceDiff(path) {
  try {
    const response = await fetch(`/api/source-diff?path=${encodeURIComponent(path)}`);
    if (!response.ok) return null;
    return await response.json();
  } catch {
    return null;
  }
}

function setDiffMarkersEnabled(enabled) {
  diffMarkersEnabled = Boolean(enabled && activeSourceDiff && activeSourceDiff.available);
  applyDiffAnnotations();
  renderDocGuide(activeSourceHeadings);
  bindDocGuideLinks();
  updateDiffToggle();
  bindDiffMarkers();
}

function updateDiffToggle() {
  if (!docDiffToggle) return;
  const available = Boolean(activeSourceDiff && activeSourceDiff.available && list(activeSourceDiff.hunks).length > 0);
  docDiffToggle.classList.toggle("hidden", !available);
  docDiffToggle.classList.toggle("active", available && diffMarkersEnabled);
  docDiffToggle.textContent = available && diffMarkersEnabled ? t("diff.hide") : t("diff.show");
  docDiffToggle.disabled = !available;
}

function applyDiffAnnotations() {
  sourceRendered.querySelectorAll(".diff-marker-row").forEach((node) => node.remove());
  sourceRendered.classList.toggle("diff-enabled", Boolean(diffMarkersEnabled));
  if (!diffMarkersEnabled || !activeSourceDiff || !activeSourceDiff.available) return;

  const blocks = sourceBlocks();
  const hunks = list(activeSourceDiff.hunks);
  hunks.forEach((hunk, index) => {
    const anchor = anchorLineForHunk(hunk);
    const target = targetBlockForLine(blocks, anchor);
    const row = document.createElement("div");
    row.className = "diff-marker-row";
    row.dataset.diffIndex = String(index);
    row.innerHTML = renderDiffMarker(hunk, index);
    if (target && hunkIsDeleteOnly(hunk)) {
      target.before(row);
    } else if (target) {
      target.before(row);
    } else {
      sourceRendered.appendChild(row);
    }
  });
}

function sourceBlocks() {
  return [...sourceRendered.querySelectorAll("[data-source-start]")]
    .filter((node) => !node.closest(".diff-marker-row"))
    .map((node) => ({
      node,
      start: Number(node.dataset.sourceStart || 0),
      end: Number(node.dataset.sourceEnd || node.dataset.sourceStart || 0)
    }))
    .filter((item) => item.start > 0)
    .sort((left, right) => left.start - right.start);
}

function targetBlockForLine(blocks, line) {
  if (blocks.length === 0) return null;
  return (blocks.find((block) => block.start <= line && block.end >= line)
    || blocks.find((block) => block.start >= line)
    || blocks[blocks.length - 1]).node;
}

function anchorLineForHunk(hunk) {
  const lines = list(hunk.lines);
  const firstInsert = lines.find((line) => line.type === "insert" && Number(line.candidate_line || 0) > 0);
  if (firstInsert) return Number(firstInsert.candidate_line);

  const firstDeleteIndex = lines.findIndex((line) => line.type === "delete");
  if (firstDeleteIndex >= 0) {
    const nextSurvivingLine = lines.slice(firstDeleteIndex + 1)
      .find((line) => line.type === "equal" && Number(line.candidate_line || 0) > 0);
    if (nextSurvivingLine) return Number(nextSurvivingLine.candidate_line);
  }

  const candidateLine = lines.map((line) => Number(line.candidate_line || 0)).find((line) => line > 0);
  return candidateLine || Number(hunk.candidate_start || 1);
}

function hunkIsDeleteOnly(hunk) {
  const lines = list(hunk.lines);
  return lines.some((line) => line.type === "delete") && !lines.some((line) => line.type === "insert");
}

function hunkChangeType(hunk) {
  const lines = list(hunk.lines);
  const hasInsert = lines.some((line) => line.type === "insert");
  const hasDelete = lines.some((line) => line.type === "delete");
  if (hasInsert && hasDelete) return "modified";
  if (hasInsert) return "added";
  if (hasDelete) return "deleted";
  return "context";
}

function renderDiffMarker(hunk, index) {
  const type = hunkChangeType(hunk);
  const expanded = expandedDiffMarkers.has(String(index));
  const summary = summarizeDiffHunk(hunk);
  return `
    <button class="diff-marker ${escapeAttr(type)}" type="button" data-diff-toggle="${escapeAttr(index)}" aria-expanded="${escapeAttr(expanded)}">
      <span class="diff-marker-type">${escapeHTML(t(`diff.${type}`))}</span>
      <span class="diff-marker-body">
        <span class="diff-marker-title">
          <strong>${escapeHTML(diffRangeSummary(summary))}</strong>
          <em>${escapeHTML(diffCountSummary(summary))}</em>
        </span>
        <span class="diff-marker-action">${escapeHTML(t("diff.expand"))}</span>
      </span>
    </button>
    ${expanded ? renderDiffHunk(hunk) : ""}
  `;
}

function summarizeDiffHunk(hunk) {
  const changedLines = list(hunk.lines).filter((line) => line.type === "insert" || line.type === "delete");
  const inserted = changedLines.filter((line) => line.type === "insert");
  const deleted = changedLines.filter((line) => line.type === "delete");
  return {
    inserted,
    deleted,
    stableRange: lineRangeText(deleted.map((line) => Number(line.stable_line || 0)).filter(Boolean)),
    candidateRange: lineRangeText(inserted.map((line) => Number(line.candidate_line || 0)).filter(Boolean))
  };
}

function lineRangeText(lines) {
  if (lines.length === 0) return "";
  const min = Math.min(...lines);
  const max = Math.max(...lines);
  return min === max ? `L${min}` : `L${min}-L${max}`;
}

function diffRangeSummary(summary) {
  const parts = [];
  if (summary.stableRange) parts.push(`${t("diff.stableRange")} ${summary.stableRange}`);
  if (summary.candidateRange) parts.push(`${t("diff.candidateRange")} ${summary.candidateRange}`);
  return parts.length > 0 ? parts.join(" -> ") : t("diff.summary");
}

function diffCountSummary(summary) {
  const parts = [];
  if (summary.deleted.length > 0) parts.push(t("diff.deletedLines", { count: summary.deleted.length }));
  if (summary.inserted.length > 0) parts.push(t("diff.insertedLines", { count: summary.inserted.length }));
  return parts.join(" · ");
}

function renderDiffHunk(hunk) {
  return `
    <div class="diff-hunk" role="region" aria-label="${escapeAttr(t("diff.summary"))}">
      ${list(hunk.lines).map((line) => {
        const lineNo = line.type === "delete" ? line.stable_line : line.candidate_line;
        return `
          <div class="diff-line ${escapeAttr(line.type)}">
            <span class="diff-line-no">${escapeHTML(lineNo || "")}</span>
            <span class="diff-line-prefix">${escapeHTML(diffLinePrefix(line.type))}</span>
            <code>${escapeHTML(line.text || "")}</code>
          </div>
        `;
      }).join("")}
    </div>
  `;
}

function diffLinePrefix(type) {
  if (type === "insert") return "+";
  if (type === "delete") return "-";
  return " ";
}

function bindDiffMarkers() {
  sourceRendered.querySelectorAll("[data-diff-toggle]").forEach((button) => {
    button.addEventListener("click", () => {
      const index = button.dataset.diffToggle;
      if (expandedDiffMarkers.has(index)) {
        expandedDiffMarkers.delete(index);
      } else {
        expandedDiffMarkers.add(index);
      }
      applyDiffAnnotations();
      bindDiffMarkers();
    });
  });
}

function renderMarkdown(markdown) {
  return renderMarkdownDocument(markdown).html;
}

function renderMarkdownDocument(markdown) {
  const parsed = splitFrontmatter(String(markdown || "").replaceAll("\r\n", "\n"));
  const lines = parsed.body.split("\n");
  const html = [];
  const headings = [];
  let paragraph = [];
  let paragraphStartLine = 0;
  let listType = "";
  let inCode = false;
  let codeLines = [];
  let codeLang = "";
  let codeStartLine = 0;
  let tableLines = [];

  const flushParagraph = () => {
    if (paragraph.length === 0) return;
    const startLine = paragraphStartLine || paragraph[0].line;
    const endLine = paragraph[paragraph.length - 1].line;
    html.push(`<p data-source-start="${escapeAttr(startLine)}" data-source-end="${escapeAttr(endLine)}">${renderInline(paragraph.map((item) => item.text).join(" "))}</p>`);
    paragraph = [];
    paragraphStartLine = 0;
  };
  const flushList = () => {
    if (!listType) return;
    html.push(`</${listType}>`);
    listType = "";
  };
  const flushTable = () => {
    if (tableLines.length === 0) return;
    html.push(renderTable(tableLines));
    tableLines = [];
  };
  const flushBlocks = () => {
    flushParagraph();
    flushList();
    flushTable();
  };

  if (parsed.frontmatter.length > 0) {
    html.push(renderFrontmatter(parsed.frontmatter, 1, parsed.bodyStartLine - 2));
  }

  let skipUntilIndex = 0;
  lines.forEach((line, index) => {
    if (index < skipUntilIndex) return;
    const sourceLine = parsed.bodyStartLine + index;
    if (line.startsWith("```")) {
      if (inCode) {
        const code = codeLines.join("\n");
        if (codeLang === "mermaid" || codeLang === "mmd") {
          html.push(`<div class="mermaid" data-source-start="${escapeAttr(codeStartLine)}" data-source-end="${escapeAttr(sourceLine)}">${escapeHTML(code)}</div>`);
        } else {
          html.push(`<pre data-source-start="${escapeAttr(codeStartLine)}" data-source-end="${escapeAttr(sourceLine)}"><code>${escapeHTML(code)}</code></pre>`);
        }
        codeLines = [];
        codeLang = "";
        codeStartLine = 0;
        inCode = false;
      } else {
        flushBlocks();
        inCode = true;
        codeStartLine = sourceLine;
        codeLang = line.slice(3).trim().split(/\s+/)[0].toLowerCase();
      }
      return;
    }
    if (inCode) {
      codeLines.push(line);
      return;
    }

    const trimmed = line.trim();
    if (trimmed === "acceptance_item_set:") {
      flushBlocks();
      const block = parseAcceptanceItemSet(lines, index, parsed.bodyStartLine);
      if (block) {
        html.push(renderAcceptanceItemSet(block));
        skipUntilIndex = block.endIndex + 1;
        return;
      }
    }
    if (trimmed === "") {
      flushBlocks();
      return;
    }
    if (isTableLine(trimmed)) {
      flushParagraph();
      flushList();
      tableLines.push({ text: trimmed, line: sourceLine });
      return;
    }
    flushTable();

    const heading = /^(#{1,4})\s+(.+)$/.exec(trimmed);
    if (heading) {
      flushParagraph();
      flushList();
      const level = heading[1].length;
      const text = plainHeadingText(heading[2]);
      const id = `doc-heading-${headings.length + 1}`;
      headings.push({ id, level, text, line: sourceLine });
      html.push(`<h${level} id="${escapeAttr(id)}" data-source-start="${escapeAttr(sourceLine)}" data-source-end="${escapeAttr(sourceLine)}">${renderInline(heading[2])}</h${level}>`);
      return;
    }

    const unordered = /^[-*]\s+(.+)$/.exec(trimmed);
    if (unordered) {
      flushParagraph();
      if (listType && listType !== "ul") flushList();
      if (!listType) {
        listType = "ul";
        html.push("<ul>");
      }
      html.push(`<li data-source-start="${escapeAttr(sourceLine)}" data-source-end="${escapeAttr(sourceLine)}">${renderInline(unordered[1])}</li>`);
      return;
    }

    const ordered = /^\d+\.\s+(.+)$/.exec(trimmed);
    if (ordered) {
      flushParagraph();
      if (listType && listType !== "ol") flushList();
      if (!listType) {
        listType = "ol";
        html.push("<ol>");
      }
      html.push(`<li data-source-start="${escapeAttr(sourceLine)}" data-source-end="${escapeAttr(sourceLine)}">${renderInline(ordered[1])}</li>`);
      return;
    }

    if (trimmed.startsWith("> ")) {
      flushParagraph();
      flushList();
      html.push(`<blockquote data-source-start="${escapeAttr(sourceLine)}" data-source-end="${escapeAttr(sourceLine)}">${renderInline(trimmed.slice(2))}</blockquote>`);
      return;
    }

    flushList();
    if (paragraph.length === 0) paragraphStartLine = sourceLine;
    paragraph.push({ text: trimmed, line: sourceLine });
  });

  if (inCode) {
    const endLine = parsed.bodyStartLine + lines.length - 1;
    html.push(`<pre data-source-start="${escapeAttr(codeStartLine || endLine)}" data-source-end="${escapeAttr(endLine)}"><code>${escapeHTML(codeLines.join("\n"))}</code></pre>`);
  }
  flushBlocks();
  return { html: html.join(""), headings };
}

function parseAcceptanceItemSet(lines, startIndex, bodyStartLine) {
  const items = [];
  let current = null;
  let endIndex = startIndex;

  for (let index = startIndex + 1; index < lines.length; index += 1) {
    const line = lines[index];
    const trimmed = line.trim();
    if (trimmed === "") break;
    if (!/^\s/.test(line)) break;

    const itemStart = /^\s*-\s+id:\s*(.+)$/.exec(line);
    if (itemStart) {
      current = { id: itemStart[1].trim(), fields: [], startLine: bodyStartLine + index };
      items.push(current);
      endIndex = index;
      continue;
    }

    const field = /^\s+([A-Za-z0-9_]+):\s*(.*)$/.exec(line);
    if (field && current) {
      current.fields.push({
        key: field[1],
        value: field[2].trim(),
        line: bodyStartLine + index
      });
      endIndex = index;
    }
  }

  if (items.length === 0) return null;
  return {
    items,
    startLine: bodyStartLine + startIndex,
    endLine: bodyStartLine + endIndex,
    endIndex
  };
}

function renderAcceptanceItemSet(block) {
  return `
    <section class="acceptance-item-set" data-source-start="${escapeAttr(block.startLine)}" data-source-end="${escapeAttr(block.endLine)}">
      <div class="acceptance-set-heading">
        <code>acceptance_item_set</code>
        <span>${escapeHTML(String(block.items.length))} items</span>
      </div>
      <div class="acceptance-items">
        ${block.items.map(renderAcceptanceItem).join("")}
      </div>
    </section>
  `;
}

function renderAcceptanceItem(item) {
  const fields = item.fields.filter((field) => field.key !== "id");
  const runnable = fields.find((field) => field.key === "not_runnable_yet");
  const statusClass = runnable && runnable.value === "yes" ? "not-runnable" : "runnable";
  const statusLabel = runnable && runnable.value === "yes" ? "not runnable yet" : "runnable";
  return `
    <article class="acceptance-item ${statusClass}" data-source-start="${escapeAttr(item.startLine)}" data-source-end="${escapeAttr(item.fields.length ? item.fields[item.fields.length - 1].line : item.startLine)}">
      <header>
        <code>${escapeHTML(item.id)}</code>
        <span>${escapeHTML(statusLabel)}</span>
      </header>
      <dl>
        ${fields.map((field) => `
          <dt>${escapeHTML(field.key)}</dt>
          <dd>${renderInline(field.value)}</dd>
        `).join("")}
      </dl>
    </article>
  `;
}

function renderDocGuide(headings) {
  const items = list(headings);
  if (!docGuide) return;
  if (items.length === 0) {
    docGuide.classList.add("hidden");
    docGuide.innerHTML = "";
    truthPanel.classList.add("guide-closed");
    updateDocGuideToggle();
    return;
  }
  docGuide.classList.toggle("hidden", !docGuideOpen);
  truthPanel.classList.toggle("guide-closed", !docGuideOpen);
  docGuide.innerHTML = `
    <div class="doc-guide-title">${escapeHTML(t("source.guideTitle"))}</div>
    <div class="doc-guide-list">
      ${items.map((heading, index) => `
        <button class="doc-guide-item depth-${Math.min(Math.max(heading.level, 1), 4)}" type="button" data-heading-id="${escapeAttr(heading.id)}" data-heading-line="${escapeAttr(heading.line)}">
          <span>${escapeHTML(heading.text || t("source.noGuide"))}</span>
          ${renderHeadingDiffBadge(heading, index, items)}
        </button>
      `).join("")}
    </div>
  `;
  updateDocGuideToggle();
}

function renderHeadingDiffBadge(heading, index, headings) {
  if (!diffMarkersEnabled || !activeSourceDiff || !activeSourceDiff.available) return "";
  const start = Number(heading.line || 0);
  const next = list(headings).slice(index + 1).find((item) => Number(item.level || 0) <= Number(heading.level || 0));
  const end = next ? Number(next.line || start) - 1 : Number.MAX_SAFE_INTEGER;
  const types = new Set();
  list(activeSourceDiff.hunks).forEach((hunk) => {
    const line = anchorLineForHunk(hunk);
    if (line >= start && line <= end) types.add(hunkChangeType(hunk));
  });
  if (types.size === 0) return "";
  const type = types.has("modified") ? "modified" : types.has("added") ? "added" : "deleted";
  return `<em class="doc-guide-diff ${escapeAttr(type)}">${escapeHTML(t(`diff.${type}`))}</em>`;
}

function setDocGuideOpen(open) {
  docGuideOpen = Boolean(open);
  renderDocGuide(activeSourceHeadings);
  bindDocGuideLinks();
}

function updateDocGuideToggle() {
  if (!docGuideToggle) return;
  const hasGuide = list(activeSourceHeadings).length > 0;
  docGuideToggle.disabled = !hasGuide;
  docGuideToggle.textContent = hasGuide
    ? t(docGuideOpen ? "source.guideHide" : "source.guideShow")
    : t("source.guideUnavailable");
  docGuideToggle.setAttribute("aria-expanded", String(hasGuide && docGuideOpen));
}

function bindDocGuideLinks() {
  if (!docGuide) return;
  docGuide.querySelectorAll("[data-heading-id]").forEach((button) => {
    button.addEventListener("click", () => {
      if (activeDocMode === "raw") {
        scrollRawSourceToLine(Number(button.dataset.headingLine || 1));
        return;
      }
      const target = document.getElementById(button.dataset.headingId);
      if (target) target.scrollIntoView({ block: "start", behavior: "smooth" });
    });
  });
}

function scrollRawSourceToLine(line) {
  const style = window.getComputedStyle(sourceContent);
  const lineHeight = Number.parseFloat(style.lineHeight) || 18;
  sourceContent.scrollTop = Math.max(0, (Math.max(line, 1) - 1) * lineHeight - 24);
}

function plainHeadingText(text) {
  return String(text || "")
    .replace(/\[([^\]]+)\]\([^)]+\)/g, "$1")
    .replace(/[`*_#]/g, "")
    .trim();
}

async function renderMermaidBlocks() {
  const nodes = sourceRendered.querySelectorAll(".mermaid");
  if (nodes.length === 0 || typeof mermaid === "undefined") return;
  try {
    if (!mermaidReady) {
      mermaid.initialize({
        startOnLoad: false,
        securityLevel: "strict",
        theme: "default",
        flowchart: { htmlLabels: true }
      });
      mermaidReady = true;
    }
    await mermaid.run({ nodes });
  } catch (error) {
    nodes.forEach((node) => {
      node.classList.add("mermaid-error");
    });
  }
}

function splitFrontmatter(markdown) {
  const lines = markdown.split("\n");
  if (lines[0] !== "---") {
    return { frontmatter: [], body: markdown, bodyStartLine: 1 };
  }
  const end = lines.findIndex((line, index) => index > 0 && line === "---");
  if (end < 0) {
    return { frontmatter: [], body: markdown, bodyStartLine: 1 };
  }
  return {
    frontmatter: lines.slice(1, end),
    body: lines.slice(end + 1).join("\n"),
    bodyStartLine: end + 2
  };
}

function renderFrontmatter(lines, startLine = 1, endLine = 1) {
  const rows = [];
  let current = null;
  const pushCurrent = () => {
    if (!current) return;
    rows.push(current);
    current = null;
  };

  lines.forEach((line) => {
    const keyValue = /^([A-Za-z0-9_.-]+):\s*(.*)$/.exec(line);
    if (keyValue) {
      pushCurrent();
      current = { key: keyValue[1], values: keyValue[2] ? [keyValue[2]] : [] };
      return;
    }
    const listValue = /^\s*-\s+(.+)$/.exec(line);
    if (listValue && current) {
      current.values.push(listValue[1]);
      return;
    }
    if (line.trim() && current) {
      current.values.push(line.trim());
    }
  });
  pushCurrent();

  if (rows.length === 0) {
    return `<section class="frontmatter-block" data-source-start="${escapeAttr(startLine)}" data-source-end="${escapeAttr(endLine)}"><h2>${escapeHTML(t("frontmatter.title"))}</h2><pre><code>${escapeHTML(lines.join("\n"))}</code></pre></section>`;
  }
  return `
    <section class="frontmatter-block" data-source-start="${escapeAttr(startLine)}" data-source-end="${escapeAttr(endLine)}">
      <h2>${escapeHTML(t("frontmatter.title"))}</h2>
      <table>
        ${rows.map((row) => `<tr><th>${escapeHTML(row.key)}</th><td>${row.values.length > 0 ? row.values.map(renderInline).join("<br>") : escapeHTML(t("frontmatter.undeclared"))}</td></tr>`).join("")}
      </table>
    </section>
  `;
}

function renderInline(text) {
  const placeholders = [];
  let escaped = escapeHTML(text);
  escaped = escaped.replace(/`([^`]+)`/g, (_, code) => {
    const token = `@@CODE${placeholders.length}@@`;
    placeholders.push(`<code>${code}</code>`);
    return token;
  });
  escaped = escaped.replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>");
  escaped = escaped.replace(/\[([^\]]+)\]\(([^)]+)\)/g, (_, label, href) => {
    const safeHref = escapeAttr(href);
    return `<a href="${safeHref}" data-doc-link="${safeHref}">${label}</a>`;
  });
  placeholders.forEach((value, index) => {
    escaped = escaped.replace(`@@CODE${index}@@`, value);
  });
  return escaped;
}

function isTableLine(line) {
  return line.includes("|") && line.startsWith("|") && line.endsWith("|");
}

function renderTable(lines) {
  if (lines.length < 2) {
    return lines.map((line) => `<p data-source-start="${escapeAttr(line.line)}" data-source-end="${escapeAttr(line.line)}">${renderInline(line.text)}</p>`).join("");
  }
  const rows = lines.filter((line) => !/^\|\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)+\|?$/.test(line.text));
  if (rows.length === 0) return "";
  const startLine = rows[0].line;
  const endLine = rows[rows.length - 1].line;
  return `<table data-source-start="${escapeAttr(startLine)}" data-source-end="${escapeAttr(endLine)}">${rows.map((line, rowIndex) => {
    const cells = line.text.split("|").slice(1, -1);
    const tag = rowIndex === 0 ? "th" : "td";
    return `<tr>${cells.map((cell) => `<${tag}>${renderInline(cell.trim())}</${tag}>`).join("")}</tr>`;
  }).join("")}</table>`;
}

function startSnapshotPolling() {
  window.setInterval(loadSnapshot, SNAPSHOT_POLL_INTERVAL_MS);
}

function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

function escapeAttr(value) {
  return escapeHTML(value).replaceAll("'", "&#39;");
}

function list(value) {
  return Array.isArray(value) ? value : [];
}

function bindRenderedDocLinks(currentPath) {
  sourceRendered.querySelectorAll("[data-doc-link]").forEach((link) => {
    link.addEventListener("click", (event) => {
      const targetPath = resolveDocLink(link.dataset.docLink, currentPath);
      if (!targetPath) return;
      event.preventDefault();
      navigateToSpecDocument(targetPath);
    });
  });
}

function resolveDocLink(rawHref, currentPath) {
  if (!rawHref) return "";
  const withoutHash = rawHref.split("#")[0];
  if (!withoutHash || /^[a-z]+:/i.test(withoutHash)) return "";
  if (!withoutHash.endsWith(".md")) return "";

  let resolved = withoutHash;
  if (!resolved.startsWith("docs/")) {
    const base = currentPath.split("/").slice(0, -1).join("/");
    resolved = normalizePath(`${base}/${resolved}`);
  }
  return normalizeSpecPath(resolved);
}

function normalizePath(path) {
  const stack = [];
  path.split("/").forEach((part) => {
    if (!part || part === ".") return;
    if (part === "..") {
      stack.pop();
      return;
    }
    stack.push(part);
  });
  return stack.join("/");
}

function normalizeSpecPath(path) {
  const normalized = normalizePath(path);
  const basename = normalized.split("/").pop();
  const direct = findKnownSpecPath(normalized);
  if (direct) return direct;

  const ruleMatch = /(?:^|\/)rule\/(candidate|stable)\/([^/]+\.md)$/.exec(normalized);
  if (ruleMatch) {
    const rulePath = `docs/specs/rules/${ruleMatch[1]}/${ruleMatch[2]}`;
    const knownRulePath = findKnownSpecPath(rulePath);
    if (knownRulePath) return knownRulePath;
  }

  return findKnownSpecPathByBasename(basename) || normalized;
}

function findKnownSpecPath(path) {
  if (isReadableOriginalPath(path)) {
    const exists = list(snapshot.sources).some((source) => source.path === path)
      || list(snapshot.nodes).some((node) => node.source && node.source.path === path);
    if (exists) return path;
  }
  return "";
}

function findKnownSpecPathByBasename(basename) {
  if (!basename) return "";
  const candidates = list(snapshot.sources)
    .map((source) => source.path)
    .concat(list(snapshot.nodes).map((node) => node.source && node.source.path).filter(Boolean))
    .filter((path) => path && path.endsWith(`/${basename}`));
  const unique = [...new Set(candidates)];
  return unique.length === 1 ? unique[0] : "";
}

function navigateToSpecDocument(path) {
  const graph = graphForCurrentView();
  const targetNode = graph.nodes.find((node) => node.source && node.source.path === path)
    || graph.nodes.find((node) => node.id === `file:${path}`);
  if (targetNode) {
    if (currentView === "spec" && reviewItemByID(targetNode.id)) {
      focusReviewItem(targetNode.id);
    } else {
      selectedNodeID = targetNode.id;
      renderNav();
      renderDetailForNode(targetNode.id);
      focusGraphNode(targetNode.id, 1.05);
    }
  } else {
    const sourceObject = list(snapshot.objects).find((object) => list(object.truth_paths).some((ref) => ref.path === path));
    if (sourceObject) {
      selectedNodeID = objectNodeID(sourceObject);
      renderNav();
      renderDetailForNode(selectedNodeID);
      focusGraphNode(selectedNodeID, 1.05);
    }
  }
  openSource(path);
}

applyStaticText();
loadSnapshot();
startSnapshotPolling();
setDocMode("rendered");
