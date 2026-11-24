import {
  ArrowRight,
  ArrowUpDown,
  CircleCheck,
  ClipboardCheck,
  Folder,
  FolderInput,
  FolderOpen,
  HardDrive,
  Image,
  Info,
  Languages,
  Monitor,
  Moon,
  Play,
  Sun,
  Tag,
  TriangleAlert,
  X,
} from "lucide";
import { isMyGo, onFileDrop } from "mygo-runtime";
import { For, Show, createMemo, createSignal, createStore, onSettled } from "solid-js";
import { DebrisService, events } from "../mygo";
import { Alert } from "./components/ui/alert";
import { Badge } from "./components/ui/badge";
import { Button } from "./components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "./components/ui/card";
import { Checkbox } from "./components/ui/checkbox";
import { Icon } from "./components/ui/icon";
import { Input } from "./components/ui/input";
import { Progress } from "./components/ui/progress";
import { SegmentedControl } from "./components/ui/segmented";
import { Select } from "./components/ui/select";
import { LOCALE_OPTIONS, type MessageKey, setLocale, t, useLocale } from "./i18n";
import { cycleTheme, useTheme } from "./theme";

type FormatSpec = Awaited<ReturnType<typeof DebrisService.formats>>[number];
type InspectResponse = Awaited<ReturnType<typeof DebrisService.inspect>>;
type PlanResponse = Awaited<ReturnType<typeof DebrisService.plan>>;
type ConvertResponse = Awaited<ReturnType<typeof DebrisService.convert>>;

interface ParsedNotice {
  kind: "info" | "warning";
  text: string;
  action?: "includeSuggestedTags";
}

function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const idx = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const value = bytes / 1024 ** idx;
  return `${value >= 100 || idx === 0 ? value.toFixed(0) : value.toFixed(1)} ${units[idx]}`;
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms} ms`;
  return `${(ms / 1000).toFixed(2)} s`;
}

function joinPath(parent: string, child: string): string {
  const sep = parent.includes("\\") ? "\\" : "/";
  const trimmedParent = parent.replace(/[\\/]+$/, "");
  const trimmedChild = child.replace(/^[\\/]+/, "");
  return `${trimmedParent}${sep}${trimmedChild}`;
}

function defaultLibraryNameFromPath(path: string, targetFmt: string): string {
  const base = path.replace(/[\\/]+$/, "").split(/[\\/]/).pop() ?? "";
  const stem = base.replace(/\.(library|pixcall|billfish|bf|reflib|refto)$/i, "") || "converted";
  if (targetFmt === "eagle") return `${stem}.library`;
  if (targetFmt === "refto") return `${stem}.reflib`;
  return stem;
}

function formatStage(stage: string): string {
  switch (stage) {
    case "read":
      return t("stageRead");
    case "prepare":
      return t("stagePrepare");
    case "init":
      return t("stageInit");
    case "assets":
      return t("stageAssets");
    case "files":
      return t("stageFiles");
    case "transfer":
      return t("stageTransfer");
    case "metadata":
      return t("stageMetadata");
    case "database":
      return t("stageDatabase");
    case "write":
      return t("stageWrite");
    case "done":
      return t("stageDone");
    default:
      return stage || t("progressTitle");
  }
}

const LOSS_KEY_MAP: Record<string, MessageKey> = {
  "folders are not supported by target format": "lossFolders",
  "nested folder hierarchy is not supported by target format and may be flattened": "lossNestedFolders",
  "multi-folder asset associations are not supported by target format": "lossMultiFolder",
  "tag groups are not supported by target format": "lossTagGroups",
  "nested tag hierarchy is not supported by target format and may be flattened": "lossNestedTags",
  "asset color palettes are not supported by target format": "lossColors",
  "recycle bin / deleted item state is not supported by target format": "lossRecycleBin",
  "custom thumbnails are not supported by target format": "lossThumbnails",
  "asset ratings are not supported by target format": "lossRating",
  "asset origin URLs are not supported by target format": "lossOriginURL",
  "asset annotations are not supported by target format": "lossAnnotation",
};

function formatLoss(msg: string): string {
  const key = LOSS_KEY_MAP[msg];
  return key ? t(key) : msg;
}

type DropZone = "source" | "output";

function DropOverlay(props: { show: boolean; label: string }) {
  return (
    <Show when={props.show}>
      <div class="pointer-events-none absolute inset-0 z-10 flex flex-col items-center justify-center gap-2 rounded-lg border-2 border-dashed border-primary bg-card/90 px-4 text-center backdrop-blur-[2px]">
        <span class="inline-flex h-10 w-10 items-center justify-center rounded-full bg-primary/12 text-primary">
          <Icon icon={FolderInput} size={20} />
        </span>
        <span class="text-xs font-medium text-foreground">{props.label}</span>
      </div>
    </Show>
  );
}

function resolveDropZone(target: EventTarget | null, x?: number, y?: number): DropZone | "" {
  if (target instanceof Element) {
    const zone = target.closest<HTMLElement>("[data-dropzone]")?.dataset.dropzone;
    if (zone === "source" || zone === "output") return zone;
  }
  if (x !== undefined && y !== undefined) {
    const zone = document.elementFromPoint(x, y)?.closest<HTMLElement>("[data-dropzone]")?.dataset.dropzone;
    if (zone === "source" || zone === "output") return zone;
  }
  return "";
}

function ThemeIcon(props: { mode: "system" | "light" | "dark" }) {
  const icon = () => (props.mode === "dark" ? Moon : props.mode === "light" ? Sun : Monitor);
  return <Icon icon={icon()} size={14} />;
}

export default function App() {
  const locale = useLocale();
  const theme = useTheme();

  const [formats, setFormats] = createSignal<FormatSpec[]>([]);
  const [inputPath, setInputPath] = createSignal("");
  const [outputPath, setOutputPath] = createSignal("");
  const [sourceFormat, setSourceFormat] = createSignal("");
  const [targetFormat, setTargetFormat] = createSignal("eagle");
  const [copyMode, setCopyMode] = createSignal<"auto" | "copy" | "hardlink">("auto");
  const [libraryName, setLibraryName] = createSignal("");
  const [libraryNameEdited, setLibraryNameEdited] = createSignal(false);
  const [overwrite, setOverwrite] = createSignal(false);
  const [includeDeleted, setIncludeDeleted] = createSignal(false);
  const [includeHidden, setIncludeHidden] = createSignal(false);
  const [includeSuggestedTags, setIncludeSuggestedTags] = createSignal(false);

  const [inspectInfo, setInspectInfo] = createSignal<InspectResponse | null>(null);
  const [planInfo, setPlanInfo] = createSignal<PlanResponse | null>(null);
  const [result, setResult] = createSignal<ConvertResponse | null>(null);
  const [errorMsg, setErrorMsg] = createSignal("");
  const [busyAction, setBusyAction] = createSignal<"" | "inspect" | "plan">("");
  const [isDraggingFiles, setIsDraggingFiles] = createSignal(false);
  const [activeDropZone, setActiveDropZone] = createSignal<DropZone | "">("");

  const [progress, setProgress] = createStore({
    running: false,
    stage: "",
    current: 0,
    total: 0,
    warnings: [] as string[],
  });

  const writableFormats = createMemo(() => formats().filter((f) => f.id !== "billfish"));

  function formatDisplayName(id: string): string {
    const found = formats().find((f) => f.id.toLowerCase() === id.toLowerCase());
    if (found) return found.displayName;
    if (!id) return "";
    return id.charAt(0).toUpperCase() + id.slice(1);
  }

  function formatNotice(msg: string): ParsedNotice {
    const suggestedMatch = msg.match(/^excluding (\d+) AI-suggested tag/i);
    if (suggestedMatch) {
      return {
        kind: "info",
        text: t("noticeSuggestedTags", { count: Number(suggestedMatch[1]) }),
        action: "includeSuggestedTags",
      };
    }

    const linkedMissingMatch = msg.match(/^(\d+) linked asset\(s\) are missing/i);
    if (linkedMissingMatch) {
      return { kind: "warning", text: t("noticeLinkedMissing", { count: Number(linkedMissingMatch[1]) }) };
    }

    const versionsMatch = msg.match(/^library contains (\d+) item version\(s\)/i);
    if (versionsMatch) {
      return { kind: "info", text: t("noticeVersions", { count: Number(versionsMatch[1]) }) };
    }

    const stacksMatch = msg.match(/^library contains (\d+) asset stack\(s\)/i);
    if (stacksMatch) {
      return { kind: "info", text: t("noticeStacks", { count: Number(stacksMatch[1]) }) };
    }

    const liveMatch = msg.match(/^library contains (\d+) Live Photo item\(s\)/i);
    if (liveMatch) {
      return { kind: "info", text: t("noticeLivePhotos", { count: Number(liveMatch[1]) }) };
    }

    const smartMatch = msg.match(/^library contains (\d+) smart folder\(s\)/i);
    if (smartMatch) {
      return { kind: "info", text: t("noticeSmartFolders", { count: Number(smartMatch[1]) }) };
    }

    const boardsMatch = msg.match(/^library contains (\d+) whiteboard\(s\)/i);
    if (boardsMatch) {
      return { kind: "info", text: t("noticeBoards", { count: Number(boardsMatch[1]) }) };
    }

    const noFoldersMatch = msg.match(/^(\d+) folder\(s\) will not be preserved in (.+)$/i);
    if (noFoldersMatch) {
      return {
        kind: "warning",
        text: t("noticeNoFolders", { count: Number(noFoldersMatch[1]), format: noFoldersMatch[2] }),
      };
    }

    const noTagGroupsMatch = msg.match(/^(\d+) tag group\(s\) will not be preserved in (.+)$/i);
    if (noTagGroupsMatch) {
      return {
        kind: "warning",
        text: t("noticeNoTagGroups", { count: Number(noTagGroupsMatch[1]), format: noTagGroupsMatch[2] }),
      };
    }

    const noRecycleMatch = msg.match(/^(\d+) deleted item\(s\) cannot remain in a recycle bin in (.+)$/i);
    if (noRecycleMatch) {
      return {
        kind: "warning",
        text: t("noticeNoRecycleBin", { count: Number(noRecycleMatch[1]), format: noRecycleMatch[2] }),
      };
    }

    const autoMatch = msg.match(/^auto copy mode: hardlinks avoid duplicating (\d+) bytes/i);
    if (autoMatch) {
      return { kind: "info", text: t("planInfoAuto", { size: formatBytes(Number(autoMatch[1])) }) };
    }

    const hardlinkMatch = msg.match(/estimated additional media disk usage is ~0 bytes \(vs (\d+) bytes/i);
    if (hardlinkMatch) {
      return { kind: "info", text: t("planInfoHardlink", { size: formatBytes(Number(hardlinkMatch[1])) }) };
    }

    const sameFmtMatch = msg.match(/^source and target formats are both (.+)$/i);
    if (sameFmtMatch) {
      return { kind: "warning", text: t("planWarnSameFormat", { format: formatDisplayName(sameFmtMatch[1]) }) };
    }

    return { kind: "warning", text: msg };
  }

  function resetRunState() {
    setPlanInfo(null);
    setResult(null);
    setProgress((s) => {
      s.stage = "";
      s.current = 0;
      s.total = 0;
      s.warnings = [];
    });
  }

  onSettled(() => {
    DebrisService.formats()
      .then((list) => {
        setFormats(list);
        const writable = list.filter((f) => f.id !== "billfish");
        if (writable.length > 0 && !writable.some((f) => f.id === targetFormat())) {
          setTargetFormat(writable[0].id);
        }
      })
      .catch((err) => {
        setErrorMsg(String(err));
      });

    const cleanups: (() => void)[] = [];
    let internalDrag = false;

    const clearDragState = () => {
      setIsDraggingFiles(false);
      setActiveDropZone("");
    };

    const isFileDragEvent = (e: DragEvent) =>
      !internalDrag && Boolean(e.dataTransfer && Array.from(e.dataTransfer.types).includes("Files"));

    const handleDragStart = () => {
      internalDrag = true;
    };

    const handleDragEnd = () => {
      internalDrag = false;
      clearDragState();
    };

    const handleDragOver = (e: DragEvent) => {
      if (progress.running || busyAction() || !isFileDragEvent(e)) {
        clearDragState();
        return;
      }
      setIsDraggingFiles(true);
      const zone = resolveDropZone(e.target, e.clientX, e.clientY);
      setActiveDropZone(zone);
      if (zone) {
        e.preventDefault();
        if (e.dataTransfer) {
          e.dataTransfer.dropEffect = "copy";
        }
      }
    };

    const handleDragLeave = (e: DragEvent) => {
      const nextTarget = e.relatedTarget;
      const leftWindow =
        nextTarget === null ||
        !(nextTarget instanceof Node) ||
        !document.documentElement.contains(nextTarget);
      if (leftWindow) {
        clearDragState();
      }
    };

    const handleDomDrop = (e: DragEvent) => {
      const zone = resolveDropZone(e.target, e.clientX, e.clientY);
      if (zone && isFileDragEvent(e)) {
        e.preventDefault();
      }
      clearDragState();
    };

    window.addEventListener("dragstart", handleDragStart, true);
    window.addEventListener("dragend", handleDragEnd, true);
    window.addEventListener("dragover", handleDragOver);
    window.addEventListener("dragleave", handleDragLeave);
    window.addEventListener("drop", handleDomDrop);
    window.addEventListener("blur", clearDragState);
    cleanups.push(() => {
      window.removeEventListener("dragstart", handleDragStart, true);
      window.removeEventListener("dragend", handleDragEnd, true);
      window.removeEventListener("dragover", handleDragOver);
      window.removeEventListener("dragleave", handleDragLeave);
      window.removeEventListener("drop", handleDomDrop);
      window.removeEventListener("blur", clearDragState);
    });

    if (isMyGo()) {
      cleanups.push(
        events.convertEvent.on((ev) => {
          if (ev.kind === "stage") {
            setProgress((s) => {
              s.stage = ev.stage ?? "";
            });
          } else if (ev.kind === "progress") {
            setProgress((s) => {
              if (ev.stage) s.stage = ev.stage;
              s.current = ev.current ?? 0;
              s.total = ev.total ?? 0;
            });
          } else if (ev.kind === "warning" && ev.message) {
            setProgress((s) => {
              s.warnings.push(ev.message!);
            });
          }
        })
      );

      cleanups.push(
        onFileDrop(({ paths, x, y }) => {
          clearDragState();
          if (progress.running || busyAction() || paths.length === 0) return;
          const zone = resolveDropZone(null, x, y);
          if (!zone) return;
          void handleDrop(zone, paths[0]);
        })
      );
    }

    return () => {
      for (const unlisten of cleanups) {
        unlisten();
      }
    };
  });

  async function refreshInspect(path: string, detectedFormat?: string) {
    setBusyAction("inspect");
    try {
      const info = await DebrisService.inspect({
        path,
        format: detectedFormat || undefined,
        includeDeleted: includeDeleted(),
        includeHidden: includeHidden(),
        includeSuggestedTags: includeSuggestedTags(),
      });
      setInspectInfo(info);
    } catch (err) {
      setInspectInfo(null);
      setErrorMsg(String(err));
    } finally {
      setBusyAction("");
    }
  }

  async function applySourcePath(path: string) {
    setInputPath(path);
    setSourceFormat("");
    setInspectInfo(null);
    resetRunState();

    try {
      const probed = await DebrisService.probe({ path });
      if (probed.matched) {
        setSourceFormat(probed.format);
        if (!libraryNameEdited() || !libraryName().trim()) {
          setLibraryName(defaultLibraryNameFromPath(path, targetFormat()));
          setLibraryNameEdited(false);
        }
        await refreshInspect(path, probed.format);
      } else {
        setInspectInfo(null);
        setErrorMsg(t("errUnrecognized"));
      }
    } catch (err) {
      setInspectInfo(null);
      setErrorMsg(String(err));
    }
  }

  function applyOutputPath(path: string) {
    setOutputPath(path);
    resetRunState();
  }

  async function handleDrop(zone: DropZone, rawPath: string) {
    const path = rawPath.trim();
    if (!path) return;
    setErrorMsg("");
    try {
      const isDir = await DebrisService.isDirectory(path);
      if (!isDir) {
        setErrorMsg(t("errDropNotDirectory"));
        return;
      }
      if (zone === "source") {
        await applySourcePath(path);
      } else {
        applyOutputPath(path);
      }
    } catch (err) {
      setErrorMsg(String(err));
    }
  }

  async function pickSource() {
    setErrorMsg("");
    const path = await DebrisService.pickDirectory(t("sourceEmptyTitle"));
    if (!path) return;
    await applySourcePath(path);
  }

  async function pickOutput() {
    setErrorMsg("");
    const path = await DebrisService.pickDirectory(t("outputDirLabel"));
    if (!path) return;
    applyOutputPath(path);
  }

  const resolvedOutputPath = createMemo(() => {
    const parent = outputPath().trim();
    const name = libraryName().trim();
    if (!parent || !name) return "";
    return joinPath(parent, name);
  });

  async function runPlan() {
    if (!resolvedOutputPath()) {
      setErrorMsg(t("errNeedLibraryName"));
      return;
    }
    setErrorMsg("");
    setBusyAction("plan");
    try {
      const plan = await DebrisService.plan({
        from: sourceFormat() || undefined,
        to: targetFormat(),
        inputPath: inputPath(),
        outputPath: resolvedOutputPath(),
        copyMode: copyMode(),
        overwrite: overwrite(),
        includeDeleted: includeDeleted(),
        includeHidden: includeHidden(),
        includeSuggestedTags: includeSuggestedTags(),
        libraryName: libraryName().trim() || undefined,
      });
      setPlanInfo(plan);
    } catch (err) {
      setErrorMsg(String(err));
    } finally {
      setBusyAction("");
    }
  }

  async function runConvert() {
    if (!resolvedOutputPath()) {
      setErrorMsg(t("errNeedLibraryName"));
      return;
    }
    setErrorMsg("");
    setResult(null);
    setProgress((s) => {
      s.running = true;
      s.stage = "";
      s.current = 0;
      s.total = 0;
      s.warnings = [];
    });

    try {
      const res = await DebrisService.convert({
        from: sourceFormat() || undefined,
        to: targetFormat(),
        inputPath: inputPath(),
        outputPath: resolvedOutputPath(),
        copyMode: copyMode(),
        overwrite: overwrite(),
        includeDeleted: includeDeleted(),
        includeHidden: includeHidden(),
        includeSuggestedTags: includeSuggestedTags(),
        libraryName: libraryName().trim() || undefined,
      });
      setResult(res);
    } catch (err) {
      setErrorMsg(String(err));
    } finally {
      setProgress((s) => {
        s.running = false;
      });
    }
  }

  function enableSuggestedTagsFromNotice() {
    setIncludeSuggestedTags(true);
    resetRunState();
    if (inputPath()) {
      void refreshInspect(inputPath(), sourceFormat());
    }
  }

  function buildSubStats(info: InspectResponse): string {
    const parts: string[] = [];
    if (info.tagGroupCount > 0) {
      parts.push(t("subStatsTagGroups", { count: info.tagGroupCount }));
    }
    if ((info.deletedCount ?? 0) > 0) {
      parts.push(t("subStatsDeleted", { count: info.deletedCount! }));
    }
    if ((info.hiddenCount ?? 0) > 0) {
      const key = sourceFormat() === "refto" ? "subStatsArchived" : "subStatsHidden";
      parts.push(t(key, { count: info.hiddenCount! }));
    }
    const suggestedCount = Number(info.extra?.suggestedTagCount ?? 0);
    if (suggestedCount > 0) {
      parts.push(t("subStatsSuggestedTags", { count: suggestedCount }));
    }
    return parts.join(" · ");
  }

  const percent = createMemo(() =>
    progress.total > 0 ? Math.min(100, Math.round((progress.current / progress.total) * 100)) : 0
  );

  const canRun = createMemo(() =>
    Boolean(inputPath() && resolvedOutputPath() && !progress.running && !busyAction())
  );

  const themeLabel = createMemo(() => {
    const mode = theme();
    if (mode === "light") return t("themeLight");
    if (mode === "dark") return t("themeDark");
    return t("themeSystem");
  });

  const modeHint = createMemo(() => {
    if (copyMode() === "hardlink") return t("modeHintHardlink");
    if (copyMode() === "copy") return t("modeHintCopy");
    return t("modeHintAuto");
  });

  const hiddenOptionLabel = createMemo(() =>
    sourceFormat() === "refto" ? t("optIncludeArchived") : t("optIncludeHidden")
  );

  function renderNoticeAlert(rawMsg: string) {
    const notice = formatNotice(rawMsg);
    return (
      <Alert variant={notice.kind === "info" ? "default" : "warning"} class="items-center justify-between">
        <div class="flex items-start gap-2.5">
          <Show when={notice.kind === "info"} fallback={<Icon icon={TriangleAlert} class="mt-0.5 shrink-0" />}>
            <Icon icon={Info} class="mt-0.5 shrink-0 text-muted-foreground" />
          </Show>
          <span>{notice.text}</span>
        </div>
        <Show when={notice.action === "includeSuggestedTags" && !includeSuggestedTags()}>
          <Button
            variant="outline"
            size="sm"
            disabled={progress.running}
            onClick={enableSuggestedTagsFromNotice}
            class="h-6 shrink-0 px-2 text-[11px]"
          >
            {t("actionIncludeSuggestedTags")}
          </Button>
        </Show>
      </Alert>
    );
  }

  return (
    <div class="grid h-full grid-rows-[52px_1fr_auto] overflow-hidden bg-background text-foreground">
      {/* Header */}
      <header class="flex items-center justify-between border-b border-border bg-card px-5">
        <div class="flex items-center gap-2.5">
          <span class="inline-flex h-6 w-6 items-center justify-center rounded-md bg-primary text-primary-foreground">
            <Icon icon={ArrowUpDown} size={14} strokeWidth={2} />
          </span>
          <span class="text-sm font-semibold tracking-tight">Debris</span>
          <span class="hidden text-xs text-muted-foreground sm:inline">· {t("appSubtitle")}</span>
        </div>

        <div class="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={cycleTheme}
            aria-label={themeLabel()}
            title={themeLabel()}
            class="gap-1.5"
          >
            <ThemeIcon mode={theme()} />
            <span class="hidden sm:inline">{themeLabel()}</span>
          </Button>

          <Select
            align="right"
            icon={Languages}
            ariaLabel="Language"
            value={locale()}
            options={LOCALE_OPTIONS.map((opt) => ({ value: opt.id, label: opt.label }))}
            onChange={setLocale}
          />
        </div>
      </header>

      {/* Main Content */}
      <main class="min-h-0 overflow-y-auto overscroll-contain">
        <div class="mx-auto flex w-full max-w-4xl flex-col gap-4 px-5 py-5">
          {/* Source -> Target Transfer Pair */}
          <div class="grid grid-cols-1 items-stretch gap-3 md:grid-cols-[1fr_auto_1fr]">
            {/* Source Card */}
            <Card
              data-dropzone="source"
              class={`relative flex flex-col justify-between transition-colors ${
                activeDropZone() === "source"
                  ? "border-primary ring-2 ring-primary/20"
                  : isDraggingFiles() && !progress.running
                    ? "border-primary/40 border-dashed"
                    : ""
              }`}
            >
              <DropOverlay show={activeDropZone() === "source"} label={t("dropSourceOverlay")} />
              <CardHeader class="pb-3">
                <div class="flex items-center gap-2">
                  <Icon icon={Folder} class="text-muted-foreground" />
                  <CardTitle>{t("sourceTitle")}</CardTitle>
                  <Show when={inspectInfo()}>
                    {(info) => <Badge>{formatDisplayName(info().format)}</Badge>}
                  </Show>
                </div>
                <Button variant="outline" size="sm" onClick={pickSource} disabled={progress.running}>
                  <Icon icon={FolderOpen} size={13} />
                  {inputPath() ? t("changeDir") : t("chooseDir")}
                </Button>
              </CardHeader>

              <CardContent class="flex flex-1 flex-col justify-between gap-3">
                <Show
                  when={inputPath()}
                  fallback={
                    <Button
                      variant="outline"
                      onClick={pickSource}
                      disabled={progress.running}
                      class="h-auto flex-1 flex-col gap-1 border-dashed bg-background/60 px-4 py-6 text-center whitespace-normal hover:border-primary/50 hover:bg-accent/40"
                    >
                      <Icon icon={FolderOpen} size={20} class="text-muted-foreground" />
                      <span class="mt-1 text-xs font-medium text-foreground">{t("sourceEmptyTitle")}</span>
                      <span class="text-[11.5px] font-normal text-muted-foreground">{t("sourceEmptyHint")}</span>
                    </Button>
                  }
                >
                  <div class="space-y-3">
                    <div class="rounded-md border border-border bg-background px-3 py-2">
                      <Show
                        when={inspectInfo()?.name}
                        fallback={
                          <div class="truncate text-xs text-foreground select-text" title={inputPath()}>
                            {inputPath()}
                          </div>
                        }
                      >
                        {(libName) => (
                          <div>
                            <div class="truncate text-xs font-medium text-foreground" title={libName()}>
                              {libName()}
                            </div>
                            <div
                              class="mt-0.5 truncate text-[11px] text-muted-foreground select-text"
                              title={inputPath()}
                            >
                              {inputPath()}
                            </div>
                          </div>
                        )}
                      </Show>
                    </div>

                    <Show when={inspectInfo()}>
                      {(info) => {
                        const subStats = () => buildSubStats(info());
                        return (
                          <div>
                            <div class="grid grid-cols-4 gap-2">
                              <div class="rounded-md bg-muted/60 px-2.5 py-1.5">
                                <div class="flex items-center gap-1 text-[11px] text-muted-foreground">
                                  <Icon icon={Image} size={11} />
                                  {t("metricAssets")}
                                </div>
                                <div class="text-sm font-semibold tabular-nums">{info().assetCount.toLocaleString()}</div>
                              </div>
                              <div class="rounded-md bg-muted/60 px-2.5 py-1.5">
                                <div class="flex items-center gap-1 text-[11px] text-muted-foreground">
                                  <Icon icon={Folder} size={11} />
                                  {t("metricFolders")}
                                </div>
                                <div class="text-sm font-semibold tabular-nums">{info().folderCount.toLocaleString()}</div>
                              </div>
                              <div class="rounded-md bg-muted/60 px-2.5 py-1.5">
                                <div class="flex items-center gap-1 text-[11px] text-muted-foreground">
                                  <Icon icon={Tag} size={11} />
                                  {t("metricTags")}
                                </div>
                                <div class="text-sm font-semibold tabular-nums">{info().tagCount.toLocaleString()}</div>
                              </div>
                              <div class="rounded-md bg-muted/60 px-2.5 py-1.5">
                                <div class="flex items-center gap-1 text-[11px] text-muted-foreground">
                                  <Icon icon={HardDrive} size={11} />
                                  {t("metricSize")}
                                </div>
                                <div class="text-sm font-semibold tabular-nums">{formatBytes(info().totalBytes)}</div>
                              </div>
                            </div>

                            <Show when={subStats()}>
                              <div class="mt-2 text-[11.5px] text-muted-foreground tabular-nums">{subStats()}</div>
                            </Show>
                          </div>
                        );
                      }}
                    </Show>
                  </div>
                </Show>
              </CardContent>
            </Card>

            {/* Center Connector */}
            <div class="flex items-center justify-center py-1 md:py-0">
              <span class="inline-flex h-8 w-8 items-center justify-center rounded-full border border-border bg-card text-muted-foreground shadow-xs">
                <Icon icon={ArrowRight} size={16} class="rotate-90 md:rotate-0" />
              </span>
            </div>

            {/* Target Card */}
            <Card
              data-dropzone="output"
              class={`relative flex flex-col justify-between transition-colors ${
                activeDropZone() === "output"
                  ? "border-primary ring-2 ring-primary/20"
                  : isDraggingFiles() && !progress.running
                    ? "border-primary/40 border-dashed"
                    : ""
              }`}
            >
              <DropOverlay show={activeDropZone() === "output"} label={t("dropOutputOverlay")} />
              <CardHeader class="pb-3">
                <div class="flex items-center gap-2">
                  <Icon icon={Folder} class="text-muted-foreground" />
                  <CardTitle>{t("targetTitle")}</CardTitle>
                  <Badge variant="secondary">{formatDisplayName(targetFormat())}</Badge>
                </div>
                <Button variant="outline" size="sm" onClick={pickOutput} disabled={progress.running}>
                  <Icon icon={FolderOpen} size={13} />
                  {outputPath() ? t("changeDir") : t("chooseDir")}
                </Button>
              </CardHeader>

              <CardContent class="flex flex-1 flex-col justify-between gap-3">
                <div class="space-y-3">
                  <div>
                    <div class="mb-1.5 text-[11.5px] font-medium text-muted-foreground">{t("targetFormat")}</div>
                    <SegmentedControl
                      fullWidth
                      ariaLabel={t("targetFormat")}
                      value={targetFormat()}
                      disabled={progress.running}
                      options={writableFormats().map((fmt) => ({ value: fmt.id, label: fmt.displayName }))}
                      onChange={(next) => {
                        setTargetFormat(next);
                        if (inputPath() && !libraryNameEdited()) {
                          setLibraryName(defaultLibraryNameFromPath(inputPath(), next));
                        }
                        resetRunState();
                      }}
                    />
                  </div>

                  <div>
                    <div class="mb-1.5 text-[11.5px] font-medium text-muted-foreground">{t("outputDirLabel")}</div>
                    <Show
                      when={outputPath()}
                      fallback={
                        <Button
                          variant="outline"
                          onClick={pickOutput}
                          disabled={progress.running}
                          class="h-auto w-full justify-start border-dashed bg-background/60 px-3 py-2.5 text-left font-normal text-muted-foreground hover:border-primary/50 hover:bg-accent/40"
                        >
                          {t("outputEmptyHint")}
                        </Button>
                      }
                    >
                      <div
                        class="truncate rounded-md border border-border bg-background px-2.5 py-2 text-[11.5px] text-foreground select-text"
                        title={outputPath()}
                      >
                        {outputPath()}
                      </div>
                    </Show>
                  </div>

                  <div class="space-y-1.5">
                    <div class="text-[11.5px] font-medium text-muted-foreground">{t("libraryNameLabel")}</div>
                    <Input
                      type="text"
                      placeholder={t("libraryNamePlaceholder")}
                      value={libraryName()}
                      disabled={progress.running}
                      onInput={(e) => {
                        setLibraryName(e.currentTarget.value);
                        setLibraryNameEdited(true);
                        resetRunState();
                      }}
                    />
                    <Show when={resolvedOutputPath()}>
                      {(fullPath) => (
                        <div class="truncate text-[11px] text-muted-foreground select-text" title={fullPath()}>
                          {t("resolvedOutputHint", { path: fullPath() })}
                        </div>
                      )}
                    </Show>
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>

          {/* Transfer Settings Strip */}
          <Card>
            <CardContent class="grid grid-cols-1 gap-4 pt-4 md:grid-cols-[1.15fr_1fr] md:items-center">
              <div class="space-y-1.5">
                <div class="flex items-center justify-between gap-3">
                  <span class="text-xs font-medium text-foreground">{t("transferMode")}</span>
                  <SegmentedControl
                    size="sm"
                    ariaLabel={t("transferMode")}
                    value={copyMode()}
                    disabled={progress.running}
                    options={[
                      { value: "auto", label: t("modeAuto") },
                      { value: "hardlink", label: t("modeHardlink") },
                      { value: "copy", label: t("modeCopy") },
                    ]}
                    onChange={(mode) => {
                      setCopyMode(mode);
                      resetRunState();
                    }}
                  />
                </div>
                <p class="text-[11.5px] text-muted-foreground">{modeHint()}</p>
              </div>

              <div class="grid grid-cols-2 gap-x-4 gap-y-2 border-t border-border pt-3 md:border-t-0 md:border-l md:pt-0 md:pl-4">
                <Checkbox
                  label={t("optOverwrite")}
                  checked={overwrite()}
                  disabled={progress.running}
                  onChange={(checked) => {
                    setOverwrite(checked);
                    resetRunState();
                  }}
                />
                <Checkbox
                  label={t("optIncludeDeleted")}
                  checked={includeDeleted()}
                  disabled={progress.running}
                  onChange={(checked) => {
                    setIncludeDeleted(checked);
                    resetRunState();
                    if (inputPath()) void refreshInspect(inputPath(), sourceFormat());
                  }}
                />
                <Checkbox
                  label={hiddenOptionLabel()}
                  checked={includeHidden()}
                  disabled={progress.running}
                  onChange={(checked) => {
                    setIncludeHidden(checked);
                    resetRunState();
                    if (inputPath()) void refreshInspect(inputPath(), sourceFormat());
                  }}
                />
                <Show when={sourceFormat() === "refto"}>
                  <Checkbox
                    label={t("optIncludeSuggestedTags")}
                    checked={includeSuggestedTags()}
                    disabled={progress.running}
                    onChange={(checked) => {
                      setIncludeSuggestedTags(checked);
                      resetRunState();
                      if (inputPath()) void refreshInspect(inputPath(), sourceFormat());
                    }}
                  />
                </Show>
              </div>
            </CardContent>
          </Card>

          {/* Error Alert */}
          <Show when={errorMsg()}>
            {(msg) => (
              <Alert variant="destructive">
                <Icon icon={TriangleAlert} class="mt-0.5 shrink-0" />
                <div>{msg()}</div>
              </Alert>
            )}
          </Show>

          {/* Inspect Notices (hidden once Pre-flight Preview is shown to avoid duplicates) */}
          <Show when={!planInfo() && inspectInfo()?.warnings && inspectInfo()!.warnings!.length > 0}>
            <div class="flex flex-col gap-1.5">
              <For each={inspectInfo()!.warnings}>{(w) => renderNoticeAlert(w)}</For>
            </div>
          </Show>

          {/* Pre-flight Plan Report */}
          <Show when={planInfo()}>
            {(plan) => (
              <Card>
                <CardHeader>
                  <CardTitle>
                    {t("planTitle")} · {formatDisplayName(plan().sourceFormat)} → {formatDisplayName(plan().targetFormat)}
                  </CardTitle>
                </CardHeader>
                <CardContent class="space-y-2.5">
                  <p class="text-xs text-muted-foreground tabular-nums">
                    {t("planSummary", {
                      assets: plan().assetCount.toLocaleString(),
                      size: formatBytes(plan().totalBytes),
                      folders: plan().folderCount,
                      tags: plan().tagCount,
                    })}
                  </p>

                  <Show when={plan().losses && plan().losses!.length > 0}>
                    <ul class="flex flex-col gap-1.5">
                      <For each={plan().losses}>
                        {(loss) => (
                          <li>
                            <Alert variant="warning">
                              <Icon icon={TriangleAlert} class="mt-0.5 shrink-0" />
                              <span>
                                {t("planLossPrefix")}
                                {formatLoss(loss)}
                              </span>
                            </Alert>
                          </li>
                        )}
                      </For>
                    </ul>
                  </Show>

                  <Show when={plan().warnings && plan().warnings!.length > 0}>
                    <ul class="flex flex-col gap-1.5">
                      <For each={plan().warnings}>{(w) => <li>{renderNoticeAlert(w)}</li>}</For>
                    </ul>
                  </Show>
                </CardContent>
              </Card>
            )}
          </Show>

          {/* Live Progress */}
          <Show when={progress.running || progress.total > 0}>
            <Card>
              <CardContent class="space-y-2.5 pt-4">
                <div class="flex items-center justify-between text-xs tabular-nums">
                  <span class="font-medium text-foreground">{formatStage(progress.stage)}</span>
                  <span class="text-muted-foreground">
                    {progress.current.toLocaleString()} / {progress.total.toLocaleString()} ({percent()}%)
                  </span>
                </div>
                <Progress value={progress.current} max={Math.max(progress.total, 1)} />

                <Show when={progress.warnings.length > 0}>
                  <ul class="flex flex-col gap-1.5 pt-1">
                    <For each={progress.warnings}>
                      {(w) => (
                        <li>
                          <Alert variant="warning">
                            <Icon icon={TriangleAlert} class="mt-0.5 shrink-0" />
                            <span>{w}</span>
                          </Alert>
                        </li>
                      )}
                    </For>
                  </ul>
                </Show>
              </CardContent>
            </Card>
          </Show>

          {/* Completion Banner */}
          <Show when={result()}>
            {(res) => (
              <Alert variant="success">
                <Icon icon={CircleCheck} class="mt-0.5 shrink-0" />
                <div class="tabular-nums">
                  <strong>{t("resultTitle")}</strong> —{" "}
                  {t("resultSummarySimple", {
                    duration: formatDuration(res().durationMs),
                    total: res().assetCount.toLocaleString(),
                  })}
                  {res().skippedAssets > 0
                    ? t("resultSummarySkipped", { skipped: res().skippedAssets.toLocaleString() })
                    : ""}
                </div>
              </Alert>
            )}
          </Show>
        </div>
      </main>

      {/* Footer Action Bar */}
      <footer class="flex items-center justify-between gap-4 border-t border-border bg-card px-5 py-3">
        <div class="min-w-0 truncate text-xs text-muted-foreground">
          <Show when={busyAction() === "inspect"}>{t("statusInspecting")}</Show>
          <Show when={busyAction() === "plan"}>{t("statusPlanning")}</Show>
          <Show when={!busyAction() && progress.running}>{t("statusRunning")}</Show>
          <Show when={!busyAction() && !progress.running}>
            {inputPath() && outputPath() ? t("statusReady") : t("statusNeedDirs")}
          </Show>
        </div>

        <div class="flex shrink-0 items-center gap-2">
          <Button variant="outline" disabled={!canRun()} onClick={runPlan}>
            <Icon icon={ClipboardCheck} size={14} />
            {t("btnPlan")}
          </Button>

          <Show when={!progress.running}>
            <Button disabled={!canRun()} onClick={runConvert}>
              <Icon icon={Play} size={13} />
              {t("btnConvert")}
            </Button>
          </Show>

          <Show when={progress.running}>
            <Button variant="destructive" onClick={() => DebrisService.cancelConvert()}>
              <Icon icon={X} size={14} />
              {t("btnCancel")}
            </Button>
          </Show>
        </div>
      </footer>
    </div>
  );
}
