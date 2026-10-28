import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { App, Button, Tag } from "antd";
import { ArrowLeft, AudioLines, Check, Copy, Eye, FileText, GitBranch, Image as ImageIcon, LoaderCircle, Minus, MousePointer2, Plus, Video } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import ReactMarkdown from "react-markdown";
import { Link, useNavigate, useParams } from "react-router";
import { getCanvasWorkflow, importCanvasWorkflow } from "@/services/api/canvas-workflows";
import { loadCanvasProjectForEditing } from "@/services/user-data-sync";
import { CanvasNodeType, type CanvasNodeData, type CanvasNodeMetadata } from "@/types/canvas";
import { FullScreenLoader } from "@/components/ui/aceternity/full-screen-loader";
import { WorkspaceState } from "@/components/layout/workspace-state";

type WorkflowPreviewMetadata = CanvasNodeMetadata & {
    workflowSource?: {
        provider: "aiqianjue";
        shareToken: string;
        nodeId: string;
        type: string;
        taskType: string;
        modelCode: string;
        promptMasked: boolean;
    };
    workflowGenerationWarning?: string;
    workflowSample?: true;
    poster?: string;
    thumbnail?: string;
    cover?: string;
    description?: string;
    frame?: { description?: string };
};

function WorkflowGraph({ nodes, connections }: { nodes: CanvasNodeData[]; connections: Array<{ id: string; fromNodeId: string; toNodeId: string }> }) {
    const [selected, setSelected] = useState<string | null>(null);
    const [viewport, setViewport] = useState({ x: 24, y: 24, scale: 1 });
    const drag = useRef<{ x: number; y: number; originX: number; originY: number } | null>(null);
    const containerRef = useRef<HTMLDivElement | null>(null);
    const byId = useMemo(() => new Map(nodes.map((node) => [node.id, node])), [nodes]);
    const selectedNode = selected ? byId.get(selected) : undefined;
    const width = Math.max(1560, ...nodes.map((node) => node.position.x + node.width + 120));
    const height = Math.max(1100, ...nodes.map((node) => node.position.y + node.height + 120));
    const previewMetadata = selectedNode?.metadata as WorkflowPreviewMetadata | undefined;
    const selectedPrompt = String(selectedNode?.metadata?.composerContent || selectedNode?.metadata?.prompt || "");

    const zoomAt = (nextScale: number) => setViewport((current) => ({ ...current, scale: Math.min(2, Math.max(0.35, nextScale)) }));

    // fitView：按容器实际尺寸计算缩放与偏移，让所有节点在首屏内完整可见并居中。
    const fitView = () => {
        const container = containerRef.current;
        if (!container) return;
        const pad = 24;
        const availableWidth = Math.max(1, container.clientWidth - pad * 2);
        const availableHeight = Math.max(1, container.clientHeight - pad * 2);
        const scale = Math.min(1, availableWidth / width, availableHeight / height);
        setViewport({
            x: (container.clientWidth - width * scale) / 2,
            y: (container.clientHeight - height * scale) / 2,
            scale: Math.max(0.35, scale),
        });
    };

    // 节点数据就绪后（或容器尺寸变化时）自动适配一次；用户手动操作后不再打扰。
    const userAdjusted = useRef(false);
    useLayoutEffect(() => {
        if (!nodes.length) return;
        fitView();
        const container = containerRef.current;
        if (!container || typeof ResizeObserver === "undefined") return;
        const observer = new ResizeObserver(() => {
            if (!userAdjusted.current) fitView();
        });
        observer.observe(container);
        return () => observer.disconnect();
        // width/height 由 nodes 派生，nodes 变化即重算
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [nodes, width, height]);

    const markUserAdjusted = () => {
        userAdjusted.current = true;
    };

    return (
        <div
            ref={containerRef}
            className="relative min-h-[560px] flex-1 touch-none overflow-hidden rounded-2xl border border-[#e4e7ec] bg-[#f6f7f9]"
            aria-label="只读工作流画布"
            onPointerDown={(event) => {
                if ((event.target as HTMLElement).closest("button, video, audio")) return;
                markUserAdjusted();
                drag.current = { x: event.clientX, y: event.clientY, originX: viewport.x, originY: viewport.y };
                event.currentTarget.setPointerCapture(event.pointerId);
            }}
            onPointerMove={(event) => {
                // setViewport 的 updater 可能在后续渲染阶段才执行，此时 drag 可能已被
                // onPointerUp 置空；先取快照，updater 只读局部常量，避免 null.originX。
                const start = drag.current;
                if (!start) return;
                const offsetX = event.clientX - start.x;
                const offsetY = event.clientY - start.y;
                setViewport((current) => ({ ...current, x: start.originX + offsetX, y: start.originY + offsetY }));
            }}
            onPointerUp={() => {
                drag.current = null;
            }}
            onPointerCancel={() => {
                drag.current = null;
            }}
            onWheel={(event) => {
                if (event.ctrlKey || event.metaKey) {
                    event.preventDefault();
                    markUserAdjusted();
                    zoomAt(viewport.scale * (event.deltaY < 0 ? 1.08 : 0.92));
                } else if (event.shiftKey) {
                    event.preventDefault();
                    markUserAdjusted();
                    setViewport((current) => ({ ...current, x: current.x - event.deltaY }));
                }
            }}
        >
            <div className="absolute right-3 top-3 z-20 flex items-center gap-1 rounded-lg border border-[#e4e7ec] bg-white/95 p-1 shadow-sm" aria-label="预览缩放">
                <Button aria-label="缩小" title="缩小" type="text" size="small" icon={<Minus className="size-4" />} onClick={() => zoomAt(viewport.scale - 0.15)} />
                <span className="min-w-12 text-center text-xs tabular-nums text-[#69717e]">{Math.round(viewport.scale * 100)}%</span>
                <Button aria-label="放大" title="放大" type="text" size="small" icon={<Plus className="size-4" />} onClick={() => zoomAt(viewport.scale + 0.15)} />
                <Button aria-label="适配全部节点" title="适配全部节点" type="text" size="small" onClick={() => { markUserAdjusted(); fitView(); }}>
                    适配
                </Button>
            </div>
            <div
                className="absolute left-0 top-0 origin-top-left"
                style={{ width, height, transform: `translate(${viewport.x}px, ${viewport.y}px) scale(${viewport.scale})`, backgroundImage: "radial-gradient(#c9ced6 0.9px, transparent 0.9px)", backgroundSize: "22px 22px" }}
            >
                <svg className="pointer-events-none absolute inset-0" width={width} height={height} aria-hidden>
                    {connections.map((edge) => {
                        const from = byId.get(edge.fromNodeId);
                        const to = byId.get(edge.toNodeId);
                        if (!from || !to) return null;
                        const x1 = from.position.x + from.width;
                        const y1 = from.position.y + from.height / 2;
                        const x2 = to.position.x;
                        const y2 = to.position.y + to.height / 2;
                        const active = selected === edge.fromNodeId || selected === edge.toNodeId;
                        return <path key={edge.id} d={`M ${x1} ${y1} C ${x1 + 90} ${y1}, ${x2 - 90} ${y2}, ${x2} ${y2}`} fill="none" stroke={active ? "#3a7afe" : "#94a3b8"} strokeOpacity={active ? 0.75 : 0.42} strokeWidth={active ? 2.2 : 1.6} />;
                    })}
                </svg>

                {nodes.map((node) => {
                    const isImage = node.type === CanvasNodeType.Image;
                    const isVideo = node.type === CanvasNodeType.Video;
                    const isAudio = node.type === CanvasNodeType.Audio;
                    const isFrame = node.type === CanvasNodeType.Frame;
                    const metadata = node.metadata as WorkflowPreviewMetadata | undefined;
                    const src = String(node.metadata?.content || "");
                    const videoSrc = String(node.metadata?.videoPreview?.content || src);
                    const prompt = String(node.metadata?.composerContent || node.metadata?.prompt || "");
                    const text = String(node.metadata?.content || "");
                    const active = selected === node.id;
                    return (
                        <div
                            key={node.id}
                            role="button"
                            tabIndex={0}
                            onClick={() => setSelected(node.id)}
                            onKeyDown={(event) => {
                                if (event.key === "Enter" || event.key === " ") setSelected(node.id);
                            }}
                            className={`absolute rounded-[10px] transition ${isFrame ? "pointer-events-none border-2 border-dashed border-[#aab3c0]" : ""} ${active ? "ring-2 ring-[#3a7afe]/45" : ""}`}
                            style={{ left: node.position.x, top: node.position.y, width: node.width, height: node.height, zIndex: isFrame ? 0 : 1 }}
                        >
                            <div className="absolute -top-[22px] left-0 flex w-full items-center gap-2 text-[12px] font-medium leading-[1.35] text-[#68717d]">
                                <span className="truncate">{node.title}</span>
                            </div>

                            {isFrame ? (
                                <div className="pointer-events-none absolute inset-x-3 top-2 flex items-center gap-2 text-[12px] font-semibold text-[#586273]">
                                    <GitBranch className="size-3.5 shrink-0" />
                                    <span>{String(metadata?.description || metadata?.frame?.description || "分组")}</span>
                                </div>
                            ) : isImage && src ? (
                                <div className="relative size-full rounded-[10px] bg-[#e9ecf0] shadow-[0_2px_10px_rgba(15,23,42,0.08)]">
                                    <img src={src} alt={node.title} className="size-full rounded-[10px] object-contain" loading="lazy" decoding="async" draggable={false} />
                                    <span className="pointer-events-none absolute inset-0 rounded-[10px] ring-1 ring-inset ring-black/[0.06]" />
                                    <span className="absolute right-2 top-2 rounded-[8px] bg-black/65 px-2 py-1 text-[11px] font-medium leading-none text-white">{String(node.metadata?.size || "1:1")}</span>
                                </div>
                            ) : isImage ? (
                                <div className="flex size-full flex-col items-center justify-center gap-2 rounded-[10px] border border-dashed border-[#c8cfd9] bg-white text-[#98a2b3]">
                                    <ImageIcon className="size-6" />
                                    <span className="text-[12px]">待生成</span>
                                </div>
                            ) : isVideo && videoSrc ? (
                                <div className="flex size-full items-center justify-center rounded-[10px] border border-[#e2e6ec] bg-black/90 p-2">
                                    <video className="max-h-full max-w-full object-contain" src={videoSrc} poster={String(metadata?.poster || metadata?.thumbnail || metadata?.cover || "")} controls preload="metadata" aria-label={node.title} />
                                </div>
                            ) : isVideo ? (
                                <div className="flex size-full flex-col items-center justify-center gap-2 rounded-[10px] border border-dashed border-[#c8cfd9] bg-white text-[#98a2b3]">
                                    <Video className="size-6" />
                                    <span className="text-[12px]">视频待生成或未提供预览</span>
                                </div>
                            ) : isAudio ? (
                                <div className="flex size-full flex-col justify-center gap-3 rounded-[10px] border border-[#e2e6ec] bg-white p-4 shadow-[0_2px_10px_rgba(15,23,42,0.05)]">
                                    <div className="flex items-center gap-2 text-xs font-medium text-[#596273]">
                                        <AudioLines className="size-4" />
                                        音频预览
                                    </div>
                                    {src ? <audio controls preload="metadata" src={src} className="w-full" aria-label={node.title} /> : <span className="text-xs text-[#98a2b3]">音频待生成或未提供预览</span>}
                                </div>
                            ) : (
                                <div className="size-full overflow-hidden rounded-[10px] border border-[#e2e6ec] bg-white p-3 shadow-[0_2px_10px_rgba(15,23,42,0.05)]">
                                    <div className="mb-2 flex items-center gap-1.5 text-[11px] font-medium text-[#8a94a3]">
                                        <FileText className="size-3.5" />
                                        商品资料
                                    </div>
                                    <div className="max-h-full overflow-auto whitespace-pre-wrap text-[11px] leading-[1.7] text-[#6b7280]">{text || node.title}</div>
                                </div>
                            )}

                            {prompt ? <div className="pointer-events-none absolute left-0 top-full mt-1.5 w-full truncate text-[10.5px] leading-[1.4] text-[#9aa3b0]">{prompt}</div> : null}
                        </div>
                    );
                })}
            </div>

            {selectedNode ? (
                <aside
                    className="absolute bottom-3 left-3 z-10 max-h-[42%] w-[min(560px,calc(100%-1.5rem))] overflow-y-auto rounded-lg border border-[#e4e7ec] bg-white/96 p-3 text-xs shadow-[0_8px_28px_rgba(15,23,42,0.12)] backdrop-blur"
                    aria-label="节点只读详情"
                >
                    <div className="flex items-center justify-between gap-2">
                        <div className="font-medium text-[#20242c]">{selectedNode.title}</div>
                        <Tag className="!m-0">只读</Tag>
                    </div>
                    <div className="mt-1 text-[#7b8491]">
                        来源：
                        {previewMetadata?.workflowSource
                            ? `${previewMetadata.workflowSource.provider} · 节点 ${previewMetadata.workflowSource.nodeId} · ${previewMetadata.workflowSource.type} · ${previewMetadata.workflowSource.taskType} · ${previewMetadata.workflowSource.modelCode}`
                            : "工作流模板"}
                        {previewMetadata?.workflowSample ? " · 示例素材" : ""}
                    </div>
                    {previewMetadata?.workflowGenerationWarning ? <p className="mt-2 rounded bg-[#fff7e6] p-2 leading-5 text-[#8a5a00]">不可执行：{previewMetadata.workflowGenerationWarning}</p> : null}
                    {selectedPrompt ? (
                        <section className="mt-2 border-t border-[#eceef1] pt-2">
                            <div className="mb-1 flex items-center gap-1 font-medium text-[#596273]">
                                <FileText className="size-3.5" />
                                完整提示词{previewMetadata?.workflowSource?.promptMasked ? "（来源标记为脱敏，以下显示模板实际提供内容）" : ""}
                            </div>
                            <div className="whitespace-pre-wrap break-words leading-5 text-[#525b68]">{selectedPrompt}</div>
                        </section>
                    ) : (
                        <p className="mt-2 text-[#7b8491]">此节点没有提示词。导入后可在自己的画布中编辑节点配置。</p>
                    )}
                </aside>
            ) : null}
        </div>
    );
}

export default function CanvasWorkflowDetailPage() {
    const { slug = "" } = useParams();
    const navigate = useNavigate();
    const { message } = App.useApp();
    const [importing, setImporting] = useState(false);
    const query = useQuery({ queryKey: ["canvas-workflow", slug], queryFn: () => getCanvasWorkflow(slug), enabled: Boolean(slug) });
    const workflow = query.data?.workflow;
    const useWorkflow = async () => {
        if (importing) return;
        setImporting(true);
        try {
            const result = await importCanvasWorkflow(slug);
            const id = result.project.id;
            await loadCanvasProjectForEditing(id);
            message.success("已导入为你的独立画布，尚未执行生成");
            navigate(`/canvas/${id}`);
        } catch (error) {
            message.error(error instanceof Error ? error.message : "工作流导入失败");
        } finally {
            setImporting(false);
        }
    };
    if (query.isLoading) return <FullScreenLoader label="正在打开工作流" detail="读取只读节点与连线" />;
    if (query.isError || !workflow)
        return (
            <div className="grid h-full place-items-center">
                <WorkspaceState
                    icon="error"
                    title="无法打开工作流"
                    description={query.error instanceof Error ? query.error.message : "模板不存在或已下架"}
                    action={
                        <Link to="/workflows">
                            <Button>返回工作流</Button>
                        </Link>
                    }
                />
            </div>
        );
    return (
        <main className="flex h-full min-h-0 flex-col overflow-hidden bg-[#f7f8fa] text-[#20242c]">
            <header className="flex flex-wrap items-center gap-4 border-b border-[#e4e7eb] bg-white px-5 py-4 md:px-8">
                <Link to="/workflows">
                    <Button type="text" icon={<ArrowLeft />}>
                        工作流
                    </Button>
                </Link>
                <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                        <h1 className="truncate text-lg font-semibold">{workflow.name}</h1>
                        <Tag className="!m-0">{workflow.category}</Tag>
                    </div>
                    <div className="mt-1 flex items-center gap-2 text-xs text-[#858d99]">
                        <Eye className="size-3.5" />
                        只读预览 · v{workflow.version} · {workflow.nodeCount} 个节点
                    </div>
                </div>
                <Button type="primary" size="large" icon={importing ? <LoaderCircle className="animate-spin" /> : <Copy />} loading={importing} onClick={() => void useWorkflow()}>
                    导入我的画布
                </Button>
            </header>
            <div className="grid min-h-0 flex-1 gap-5 overflow-y-auto p-5 md:p-7 xl:grid-cols-[minmax(0,1fr)_280px] xl:overflow-hidden">
                <WorkflowGraph nodes={workflow.project.nodes || []} connections={workflow.project.connections || []} />
                <aside className="space-y-5 xl:overflow-y-auto">
                    {workflow.usageGuide?.trim() ? (
                        <section className="rounded-lg border border-border bg-card p-5 text-card-foreground">
                            <h2 className="text-sm font-semibold">使用指南</h2>
                            <div className="workflow-usage-guide mt-3 text-xs leading-6 text-muted-foreground">
                                <ReactMarkdown>{workflow.usageGuide}</ReactMarkdown>
                            </div>
                        </section>
                    ) : null}
                    <section className="rounded-lg border border-border bg-card p-5 text-card-foreground">
                        <h2 className="flex items-center gap-2 text-sm font-semibold">
                            <GitBranch className="size-4" />
                            如何使用
                        </h2>
                        <ol className="mt-4 space-y-4 text-xs leading-5 text-[#747d8b]">
                            {["导入为独立的新画布，不修改原模板。", "替换商品输入图，填写真实商品资料与卖点。", "按需修改每个输出节点的提示词、模型和比例。", "逐张运行生成；导入本身不会扣费或创建生成任务。"].map((text, index) => (
                                <li key={text} className="flex gap-3">
                                    <span className="grid size-5 shrink-0 place-items-center rounded-full bg-[#f0f2f5] text-[10px] text-[#626c7b]">{index + 1}</span>
                                    <span>{text}</span>
                                </li>
                            ))}
                        </ol>
                    </section>
                    <section className="rounded-lg border border-border bg-card p-5 text-card-foreground">
                        <h2 className="text-sm font-semibold">模板说明</h2>
                        <p className="mt-3 text-xs leading-6 text-[#747d8b]">{workflow.description}</p>
                        <div className="mt-4 flex items-start gap-2 rounded-xl bg-[#f3f6f4] p-3 text-[11px] leading-5 text-[#65746a]">
                            <Check className="mt-0.5 size-3.5 shrink-0" />
                            导入后节点、连线和提示词均可独立修改。
                        </div>
                    </section>
                    <div className="flex items-center gap-2 px-1 text-[11px] text-[#949ca8]">
                        <MousePointer2 className="size-3.5" />
                        点击节点可查看只读说明
                    </div>
                </aside>
            </div>
        </main>
    );
}
