import i18next from "i18next";
import LanguageDetector from "i18next-browser-languagedetector";
import { createSignal } from "solid-js";

export type Locale = "zh-Hans" | "zh-Hant" | "en";

export const LOCALE_OPTIONS: { id: Locale; label: string }[] = [
  { id: "zh-Hans", label: "简体中文" },
  { id: "zh-Hant", label: "繁體中文" },
  { id: "en", label: "English" },
];

const resources = {
  "zh-Hans": {
    translation: {
      appSubtitle: "设计素材库跨格式迁移",
      sourceTitle: "源素材库",
      sourceEmptyTitle: "选择源素材库目录",
      sourceEmptyHint: "支持 Billfish、Eagle、Pixcall 目录，选择后自动识别格式",
      chooseDir: "选择目录",
      changeDir: "更换",
      targetTitle: "目标输出",
      targetFormat: "目标格式",
      outputDirLabel: "保存位置",
      outputEmptyHint: "选择新素材库的存放父目录",
      libraryNameLabel: "目标库名称（文件夹名）",
      libraryNamePlaceholder: "例如：TestBl",
      resolvedOutputHint: "将生成：{{path}}",
      errNeedLibraryName: "请输入目标库名称。",
      transferMode: "传输模式",
      modeAuto: "自动",
      modeHardlink: "硬链接",
      modeCopy: "复制",
      modeHintAuto: "优先硬链接节省空间，跨磁盘自动回退为复制",
      modeHintHardlink: "仅创建硬链接，不占用额外空间（需同磁盘）",
      modeHintCopy: "完整复制所有素材文件，最稳妥但占用双倍空间",
      optionsTitle: "迁移选项",
      optOverwrite: "覆盖已有目标目录",
      optIncludeDeleted: "包含回收站项目",
      optIncludeHidden: "包含隐藏项目",
      metricAssets: "资产",
      metricFolders: "文件夹",
      metricTags: "标签",
      metricSize: "体积",
      subStats: "标签组 {{groups}}",
      subStatsDeleted: "回收站 {{count}}",
      subStatsHidden: "隐藏 {{count}}",
      planTitle: "转换预检",
      planSummary: "预计迁移 {{assets}} 项资产（{{size}}），包含 {{folders}} 个文件夹与 {{tags}} 个标签。",
      planLossPrefix: "兼容损耗：",
      progressTitle: "正在迁移",
      resultTitle: "迁移完成",
      resultSummary: "耗时 {{duration}} · 共处理 {{total}} 项（复制 {{copied}} / 硬链接 {{hardlinked}} / 跳过 {{skipped}}）",
      statusInspecting: "正在读取源库信息…",
      statusPlanning: "正在计算预检方案…",
      statusRunning: "正在执行转换，可随时取消…",
      statusReady: "就绪 — 可先预检或直接开始转换",
      statusNeedDirs: "请先选择源素材库与输出目录",
      btnPlan: "预检",
      btnConvert: "开始转换",
      btnCancel: "取消",
      errUnrecognized: "未能识别该目录的素材库格式，请确认选择的是有效的库根目录。",
      stageRead: "读取源库",
      stagePrepare: "准备目录",
      stageInit: "初始化目标库",
      stageAssets: "迁移素材文件",
      stageFiles: "迁移素材文件",
      stageTransfer: "迁移素材文件",
      stageMetadata: "写入库元数据",
      stageDatabase: "写入数据库",
      stageWrite: "写入目标库",
      stageDone: "收尾完成",
      planInfoAuto: "自动模式：同磁盘将优先创建硬链接（可节省 {{size}}），跨磁盘自动回退为复制。",
      planInfoHardlink: "硬链接模式：预计不额外占用磁盘空间（可节省 {{size}}），要求源与目标位于同一磁盘。",
      planWarnSameFormat: "源格式与目标格式均为 {{format}}。",
      themeSystem: "跟随系统",
      themeLight: "浅色模式",
      themeDark: "深色模式",
    },
  },
  "zh-Hant": {
    translation: {
      appSubtitle: "設計素材庫跨格式遷移",
      sourceTitle: "來源素材庫",
      sourceEmptyTitle: "選擇來源素材庫目錄",
      sourceEmptyHint: "支援 Billfish、Eagle、Pixcall 目錄，選擇後自動辨識格式",
      chooseDir: "選擇目錄",
      changeDir: "更換",
      targetTitle: "目標輸出",
      targetFormat: "目標格式",
      outputDirLabel: "儲存位置",
      outputEmptyHint: "選擇新素材庫的存放父目錄",
      libraryNameLabel: "目標庫名稱（資料夾名）",
      libraryNamePlaceholder: "例如：TestBl",
      resolvedOutputHint: "將產生：{{path}}",
      errNeedLibraryName: "請輸入目標庫名稱。",
      transferMode: "傳輸模式",
      modeAuto: "自動",
      modeHardlink: "硬連結",
      modeCopy: "複製",
      modeHintAuto: "優先使用硬連結節省空間，跨磁碟自動改為複製",
      modeHintHardlink: "僅建立硬連結，不佔用額外空間（需同磁碟）",
      modeHintCopy: "完整複製所有素材檔案，最穩妥但佔用雙倍空間",
      optionsTitle: "遷移選項",
      optOverwrite: "覆寫既有目標目錄",
      optIncludeDeleted: "包含垃圾桶項目",
      optIncludeHidden: "包含隱藏項目",
      metricAssets: "資產",
      metricFolders: "資料夾",
      metricTags: "標籤",
      metricSize: "大小",
      subStats: "標籤群組 {{groups}}",
      subStatsDeleted: "垃圾桶 {{count}}",
      subStatsHidden: "隱藏 {{count}}",
      planTitle: "轉換預檢",
      planSummary: "預計遷移 {{assets}} 項資產（{{size}}），包含 {{folders}} 個資料夾與 {{tags}} 個標籤。",
      planLossPrefix: "相容性損耗：",
      progressTitle: "正在遷移",
      resultTitle: "遷移完成",
      resultSummary: "耗時 {{duration}} · 共處理 {{total}} 項（複製 {{copied}} / 硬連結 {{hardlinked}} / 略過 {{skipped}}）",
      statusInspecting: "正在讀取來源庫資訊…",
      statusPlanning: "正在計算預檢方案…",
      statusRunning: "正在執行轉換，可隨時取消…",
      statusReady: "就緒 — 可先預檢或直接開始轉換",
      statusNeedDirs: "請先選擇來源素材庫與輸出目錄",
      btnPlan: "預檢",
      btnConvert: "開始轉換",
      btnCancel: "取消",
      errUnrecognized: "未能辨識該目錄的素材庫格式，請確認選擇的是有效的庫根目錄。",
      stageRead: "讀取來源庫",
      stagePrepare: "準備目錄",
      stageInit: "初始化目標庫",
      stageAssets: "遷移素材檔案",
      stageFiles: "遷移素材檔案",
      stageTransfer: "遷移素材檔案",
      stageMetadata: "寫入庫中繼資料",
      stageDatabase: "寫入資料庫",
      stageWrite: "寫入目標庫",
      stageDone: "收尾完成",
      planInfoAuto: "自動模式：同磁碟將優先建立硬連結（可節省 {{size}}），跨磁碟自動改為複製。",
      planInfoHardlink: "硬連結模式：預計不額外佔用磁碟空間（可節省 {{size}}），要求來源與目標位於同一磁碟。",
      planWarnSameFormat: "來源格式與目標格式均為 {{format}}。",
      themeSystem: "跟隨系統",
      themeLight: "淺色模式",
      themeDark: "深色模式",
    },
  },
  en: {
    translation: {
      appSubtitle: "Cross-format design library converter",
      sourceTitle: "Source Library",
      sourceEmptyTitle: "Select source library folder",
      sourceEmptyHint: "Supports Billfish, Eagle, and Pixcall — format is detected automatically",
      chooseDir: "Browse…",
      changeDir: "Change",
      targetTitle: "Target Output",
      targetFormat: "Target Format",
      outputDirLabel: "Save Location",
      outputEmptyHint: "Choose the parent folder for the new library",
      libraryNameLabel: "Library Name (Folder)",
      libraryNamePlaceholder: "e.g. TestBl",
      resolvedOutputHint: "Will create: {{path}}",
      errNeedLibraryName: "Please enter a target library name.",
      transferMode: "Transfer Mode",
      modeAuto: "Auto",
      modeHardlink: "Hardlink",
      modeCopy: "Copy",
      modeHintAuto: "Prefers hardlinks to save disk space; falls back to copy across volumes",
      modeHintHardlink: "Hardlinks only — zero extra disk usage (same volume required)",
      modeHintCopy: "Full file copy — safest across drives, uses additional disk space",
      optionsTitle: "Options",
      optOverwrite: "Overwrite target",
      optIncludeDeleted: "Include trashed",
      optIncludeHidden: "Include hidden",
      metricAssets: "Assets",
      metricFolders: "Folders",
      metricTags: "Tags",
      metricSize: "Size",
      subStats: "Tag groups {{groups}}",
      subStatsDeleted: "Trashed {{count}}",
      subStatsHidden: "Hidden {{count}}",
      planTitle: "Pre-flight Plan",
      planSummary: "Projected {{assets}} assets ({{size}}), {{folders}} folders, and {{tags}} tags.",
      planLossPrefix: "Compatibility loss: ",
      progressTitle: "Converting",
      resultTitle: "Conversion Complete",
      resultSummary: "Finished in {{duration}} · {{total}} assets ({{copied}} copied, {{hardlinked}} hardlinked, {{skipped}} skipped)",
      statusInspecting: "Inspecting source library…",
      statusPlanning: "Running pre-flight plan…",
      statusRunning: "Converting… you can cancel anytime",
      statusReady: "Ready — run a pre-flight plan or start converting",
      statusNeedDirs: "Select both source library and output folder to begin",
      btnPlan: "Plan",
      btnConvert: "Convert",
      btnCancel: "Cancel",
      errUnrecognized: "Could not recognize a supported library format in this folder.",
      stageRead: "Reading source library",
      stagePrepare: "Preparing target",
      stageInit: "Initializing library",
      stageAssets: "Transferring assets",
      stageFiles: "Transferring assets",
      stageTransfer: "Transferring assets",
      stageMetadata: "Writing metadata",
      stageDatabase: "Writing database",
      stageWrite: "Writing target library",
      stageDone: "Finalizing",
      planInfoAuto: "Auto mode: hardlinks save {{size}} on the same volume, or fall back to copy across volumes.",
      planInfoHardlink: "Hardlink mode: ~0 extra disk usage (saves {{size}}); source and target must share a volume.",
      planWarnSameFormat: "Source and target formats are both {{format}}.",
      themeSystem: "System theme",
      themeLight: "Light mode",
      themeDark: "Dark mode",
    },
  },
} as const;

export type MessageKey = keyof (typeof resources)["en"]["translation"];

function normalizeLocale(lng?: string): Locale {
  const lower = (lng ?? "").toLowerCase();
  if (lower.startsWith("zh")) {
    if (lower.includes("tw") || lower.includes("hk") || lower.includes("mo") || lower.includes("hant")) {
      return "zh-Hant";
    }
    return "zh-Hans";
  }
  return "en";
}

i18next.use(LanguageDetector).init({
  resources,
  fallbackLng: "en",
  supportedLngs: ["zh-Hans", "zh-Hant", "en"],
  detection: {
    order: ["localStorage", "navigator"],
    lookupLocalStorage: "debris.locale",
    caches: ["localStorage"],
  },
  interpolation: {
    escapeValue: false,
  },
});

const [locale, setLocaleSignal] = createSignal<Locale>(normalizeLocale(i18next.resolvedLanguage ?? i18next.language));

i18next.on("languageChanged", (lng) => {
  setLocaleSignal(normalizeLocale(lng));
});

export function useLocale() {
  return locale;
}

export function setLocale(next: Locale) {
  void i18next.changeLanguage(next);
}

export function t(key: MessageKey, vars?: Record<string, string | number>): string {
  locale(); // subscribe to reactive language changes
  return i18next.t(key, vars);
}
