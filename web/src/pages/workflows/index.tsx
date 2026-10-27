import { useMemo, useState } from "react";
import { Button, Input, Tag } from "antd";
import { ArrowRight, AudioLines, GitBranch, Images, RotateCcw, Search, ShieldCheck, SlidersHorizontal, Sparkles, Video, X } from "lucide-react";
import { useQueries, useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router";
import { PaginationBar } from "@/components/layout/workspace-page";
import { Select } from "@/components/ui/base/select";
import { SegmentedControl } from "@/components/ui/base/segmented-control";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { CanvasNodeType } from "@/types/canvas";
import { getCanvasWorkflow, listCanvasWorkflows, type CanvasWorkflowSummary } from "@/services/api/canvas-workflows";
import { apiBaseURL } from "@/services/api/request";
import { filterCanvasWorkflowCatalog, paginateCanvasWorkflowCatalog, type CanvasWorkflowMediaFilter, type CanvasWorkflowSort } from "@/lib/canvas-workflow-catalog";
import { FullScreenLoader } from "@/components/ui/aceternity/full-screen-loader";
import { WorkspaceState } from "@/components/layout/workspace-state";
import "./workflows.css";

const MEDIA_LABEL: Record<Exclude<CanvasWorkflowMediaFilter, "all">, string> = {
    image: "图片",
    video: "视频",
    audio: "音频",
    text: "文本",
};

function mediaIcon(type: string) {
    if (type === "video") return <Video className="size-3.5" />;
    if (type === "audio") return <AudioLines className="size-3.5" />;
    if (type === "image") return <Images className="size-3.5" />;
    return <GitBranch className="size-3.5" />;
}

function coverURL(item: CanvasWorkflowSummary) {
    if (!item.coverUrl) return "";
    const base = String(apiBaseURL).replace(/\/+$/, "");
    if (item.coverUrl.startsWith("/api/") && base !== "/api") return base + item.coverUrl.slice(4);
    return item.coverUrl;
}

export default function CanvasWorkflowsPage() {
    const navigate = useNavigate();
    const [search, setSearch] = useState("");
    const debouncedSearch = useDebouncedValue(search, 250);
    const [category, setCategory] = useState("全部");
    const [mediaType, setMediaType] = useState<CanvasWorkflowMediaFilter>("all");
    const [sort, setSort] = useState<CanvasWorkflowSort>("recent");
    const [filtersOpen, setFiltersOpen] = useState(false);
    const [page, setPage] = useState(1);
    const query = useQuery({ queryKey: ["canvas-workflows"], queryFn: listCanvasWorkflows });
    const workflows = query.data?.workflows || [];

    const fallbackItems = useMemo(() => workflows.filter((item) => !item.coverUrl), [workflows]);
    const fallbackQueries = useQueries({
        queries: fallbackItems.map((item) => ({
            queryKey: ["canvas-workflow", item.slug],
            queryFn: () => getCanvasWorkflow(item.slug),
            staleTime: 5 * 60 * 1000,
        })),
    });
    const fallbackCovers = useMemo(() => {
        const result = new Map<string, { src: string; kind: "image" | "video"; poster?: string }>();
        fallbackItems.forEach((item, index) => {
            const nodes = fallbackQueries[index]?.data?.workflow.project.nodes || [];
            const image = nodes.find((node) => node.type === CanvasNodeType.Image && node.metadata?.content);
            const video = nodes.find((node) => node.type === CanvasNodeType.Video && (node.metadata?.videoPreview?.content || node.metadata?.content));
            if (image?.metadata?.content) result.set(item.slug, { src: image.metadata.content, kind: "image" });
            else if (video?.metadata?.content || video?.metadata?.videoPreview?.content) {
                const videoMetadata = video.metadata as (typeof video.metadata & { posterFrameUrl?: string }) | undefined;
                result.set(item.slug, {
                    src: video.metadata?.videoPreview?.content || video.metadata?.content || "",
                    kind: "video",
                    poster: videoMetadata?.posterFrameUrl,
                });
            }
        });
        return result;
    }, [fallbackItems, fallbackQueries]);

    const visible = useMemo(
        () =>
            filterCanvasWorkflowCatalog(workflows, {
                query: debouncedSearch,
                category,
                mediaType,
                sort,
            }),
        [workflows, debouncedSearch, category, mediaType, sort],
    );
    const paged = useMemo(() => paginateCanvasWorkflowCatalog(visible, page), [visible, page]);
    const categories = useMemo(() => ["全部", ...new Set(workflows.map((item) => item.category).filter(Boolean))], [workflows]);
    const activeFilters = Number(category !== "全部") + Number(mediaType !== "all") + Number(sort !== "recent") + Number(Boolean(search.trim()));

    const resetFilters = () => {
        setSearch("");
        setCategory("全部");
        setMediaType("all");
        setSort("recent");
        setPage(1);
    };
    const changeFilter = (change: () => void) => {
        change();
        setPage(1);
    };

    if (query.isLoading) return <FullScreenLoader label="正在打开工作流" detail="读取可复用的创作模板" />;
    if (query.isError) {
        return (
            <div className="grid h-full place-items-center bg-background">
                <WorkspaceState icon="error" title="工作流加载失败" description={query.error instanceof Error ? query.error.message : "请稍后重试"} action={<Button onClick={() => void query.refetch()}>重新加载</Button>} />
            </div>
        );
    }

    return (
        <main className="workflow-catalog h-full overflow-y-auto bg-background text-foreground">
            <div className="workflow-catalog-inner">
                <header className="workflow-catalog-header">
                    <div>
                        <div className="workflow-catalog-eyebrow">
                            <GitBranch className="size-3.5" />
                            可复用的画布工作流
                        </div>
                        <h1>从成熟流程开始创作</h1>
                        <p>查看节点、连线与提示词，把模板导入为自己的画布，再替换素材并继续生成。</p>
                    </div>
                    <Link to="/canvas">
                        <Button icon={<Images className="size-4" />}>我的画布</Button>
                    </Link>
                </header>

                <div className="workflow-catalog-toolbar">
                    <SegmentedControl
                        ariaLabel="按工作流媒体类型筛选"
                        value={mediaType}
                        onChange={(value) => changeFilter(() => setMediaType(value as CanvasWorkflowMediaFilter))}
                        options={[
                            { value: "all", label: "全部" },
                            { value: "image", label: <span className="inline-flex items-center gap-1.5">{mediaIcon("image")}图片</span> },
                            { value: "video", label: <span className="inline-flex items-center gap-1.5">{mediaIcon("video")}视频</span> },
                            { value: "audio", label: <span className="inline-flex items-center gap-1.5">{mediaIcon("audio")}音频</span> },
                            { value: "text", label: <span className="inline-flex items-center gap-1.5">{mediaIcon("text")}文本</span> },
                        ]}
                        className="workflow-catalog-media"
                    />
                    <Input
                        allowClear
                        prefix={<Search className="size-4" />}
                        placeholder="搜索工作流名称、简介或分类"
                        aria-label="搜索工作流"
                        value={search}
                        onChange={(event) => changeFilter(() => setSearch(event.target.value))}
                        className="workflow-catalog-search"
                    />
                    <Button type="text" aria-expanded={filtersOpen} icon={filtersOpen ? <X className="size-4" /> : <SlidersHorizontal className="size-4" />} onClick={() => setFiltersOpen((value) => !value)}>
                        更多筛选{activeFilters ? " · " + activeFilters : ""}
                    </Button>
                    <span className="workflow-catalog-count" role="status">
                        共 {visible.length.toLocaleString("zh-CN")} 条
                    </span>
                </div>

                {filtersOpen ? (
                    <section className="workflow-catalog-filters" aria-label="工作流筛选条件">
                        <label>
                            <span>分类</span>
                            <Select value={category} onChange={(value) => changeFilter(() => setCategory(value))} options={categories.map((value) => ({ value, label: value === "全部" ? "全部分类" : value }))} ariaLabel="工作流分类" />
                        </label>
                        <label>
                            <span>排序</span>
                            <Select
                                value={sort}
                                onChange={(value) => changeFilter(() => setSort(value as CanvasWorkflowSort))}
                                options={[
                                    { value: "recent", label: "最近更新" },
                                    { value: "popular", label: "导入最多" },
                                ]}
                                ariaLabel="工作流排序"
                            />
                        </label>
                        {activeFilters ? (
                            <Button type="text" icon={<RotateCcw className="size-4" />} onClick={resetFilters}>
                                重置筛选
                            </Button>
                        ) : null}
                    </section>
                ) : null}

                {paged.items.length ? (
                    <>
                        <section className="workflow-catalog-grid" aria-label="工作流目录">
                            {paged.items.map((workflow) => {
                                const publishedCover = coverURL(workflow);
                                const fallback = fallbackCovers.get(workflow.slug);
                                return (
                                    <article className="workflow-catalog-card" key={workflow.slug}>
                                        <button type="button" className="workflow-catalog-card-open" onClick={() => navigate("/workflows/" + workflow.slug)}>
                                            <div className="workflow-catalog-cover">
                                                {publishedCover ? (
                                                    <img src={publishedCover} alt={workflow.name + " 工作流封面"} loading="lazy" decoding="async" />
                                                ) : fallback?.kind === "image" ? (
                                                    <img src={fallback.src} alt={workflow.name + " 工作流素材预览"} loading="lazy" decoding="async" />
                                                ) : fallback?.kind === "video" ? (
                                                    <video src={fallback.src} poster={fallback.poster} muted playsInline preload="metadata" aria-label={workflow.name + " 工作流视频预览"} />
                                                ) : (
                                                    <div className="workflow-catalog-cover-empty">
                                                        <GitBranch className="size-8" />
                                                        <span>工作流预览</span>
                                                    </div>
                                                )}
                                                <Tag className="workflow-catalog-category">{workflow.category || "未分类"}</Tag>
                                                <span className="workflow-catalog-node-count">{workflow.nodeCount} 节点</span>
                                            </div>
                                            <div className="workflow-catalog-card-body">
                                                <div className="workflow-catalog-title-row">
                                                    <h2>{workflow.name}</h2>
                                                    <ArrowRight className="workflow-catalog-arrow size-4" />
                                                </div>
                                                <p className="workflow-catalog-description">{workflow.description || "查看工作流节点与生成配置。"}</p>
                                                <div className="workflow-catalog-card-meta">
                                                    <span>
                                                        <ShieldCheck className="size-3.5" />
                                                        独立副本
                                                    </span>
                                                    <span>
                                                        <Sparkles className="size-3.5" />v{workflow.version}
                                                    </span>
                                                    {(workflow.importTotal || 0) > 0 ? <span className="workflow-catalog-imports">{(workflow.importTotal || 0).toLocaleString("zh-CN")} 次复用</span> : null}
                                                </div>
                                                <div className="workflow-catalog-media-tags">
                                                    {(workflow.mediaTypes || []).map((type) => (
                                                        <span key={type}>
                                                            {mediaIcon(type)}
                                                            {MEDIA_LABEL[type]}
                                                        </span>
                                                    ))}
                                                </div>
                                            </div>
                                        </button>
                                    </article>
                                );
                            })}
                        </section>
                        <PaginationBar
                            current={paged.page}
                            pageSize={paged.pageSize}
                            total={paged.total}
                            pageSizeOptions={[30]}
                            alwaysShow
                            itemLabel="个工作流"
                            onChange={(next) => {
                                setPage(next);
                                window.requestAnimationFrame(() => window.scrollTo({ top: 0, behavior: "smooth" }));
                            }}
                        />
                    </>
                ) : (
                    <WorkspaceState icon="empty" title="没有找到匹配的工作流" description="调整关键词或筛选条件后再试。" action={<Button onClick={resetFilters}>清除筛选</Button>} />
                )}
            </div>
        </main>
    );
}
