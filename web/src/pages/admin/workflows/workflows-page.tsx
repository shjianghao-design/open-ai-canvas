import { App, Button, Drawer, Form, Input, InputNumber, Modal, Popconfirm, Select, Space, Switch, Table, Tag, Tooltip } from "antd";
import type { ColumnsType } from "antd/es/table";
import { AlertTriangle, ArrowUpCircle, Eye, History, ImagePlus, Pencil, Plus, RefreshCw, Rocket, RotateCcw, Trash2, Upload } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router";

import { AdminPageFrame } from "@/pages/admin/components/admin-shell";
import { EmptyState } from "@/components/ui/product/empty-state";
import { listRemoteCanvasProjectsPage } from "@/services/api/user-data";
import {
    createCanvasWorkflowFromCanvas,
    adminCanvasWorkflowDraftCoverURL,
    discardAdminCanvasWorkflowCover,
    deleteCanvasWorkflow,
    getAdminCanvasWorkflow,
    listAdminCanvasWorkflows,
    openCanvasWorkflowEditSession,
    publishCanvasWorkflow,
    rollbackCanvasWorkflow,
    updateCanvasWorkflowMeta,
    uploadAdminCanvasWorkflowCover,
    type AdminCanvasWorkflowDetail,
    type AdminCanvasWorkflowSummary,
    type AdminCanvasWorkflowVersion,
} from "@/services/api/canvas-workflows-admin";
import { ImageCropDialog } from "@/components/image-crop-dialog";
import { cropWorkflowCover, WORKFLOW_COVER_ASPECT, WORKFLOW_COVER_HEIGHT, WORKFLOW_COVER_WIDTH } from "@/services/workflow-cover-image";
import type { ImageCropRect } from "@/lib/canvas/image-operation-plan";

export default function AdminWorkflowsPage() {
    const { message } = App.useApp();
    const navigate = useNavigate();
    const [items, setItems] = useState<AdminCanvasWorkflowSummary[]>([]);
    const [loading, setLoading] = useState(false);
    const [detail, setDetail] = useState<AdminCanvasWorkflowDetail | null>(null);
    const [detailOpen, setDetailOpen] = useState(false);
    const [createOpen, setCreateOpen] = useState(false);
    const [createForm] = Form.useForm();
    const [publishForm] = Form.useForm();
    const [canvases, setCanvases] = useState<Array<{ id: string; title: string }>>([]);
    const [catalogDescription, setCatalogDescription] = useState("");
    const [usageGuide, setUsageGuide] = useState("");
    const [coverResourceId, setCoverResourceId] = useState("");
    const [coverPreview, setCoverPreview] = useState("");
    const [coverCropSource, setCoverCropSource] = useState("");
    const [coverCropOpen, setCoverCropOpen] = useState(false);
    const [coverUploading, setCoverUploading] = useState(false);
    const coverInputRef = useRef<HTMLInputElement>(null);
    const pendingCoverIds = useRef<string[]>([]);
    const localCoverPreview = useRef("");

    const load = useCallback(async () => {
        setLoading(true);
        try {
            const result = await listAdminCanvasWorkflows();
            setItems(result.workflows || []);
        } catch (error) {
            message.error(error instanceof Error ? error.message : "工作流列表加载失败");
        } finally {
            setLoading(false);
        }
    }, [message]);

    useEffect(() => { void load(); }, [load]);

    const openDetail = useCallback(async (id: string) => {
        try {
            const result = await getAdminCanvasWorkflow(id);
            setDetail(result.workflow);
            setCatalogDescription(result.workflow.draftDescription || result.workflow.description || "");
            setUsageGuide(result.workflow.draftUsageGuide || "");
            setCoverResourceId(result.workflow.draftCoverResourceId || "");
            setCoverPreview(result.workflow.draftCoverUrl || "");
            setDetailOpen(true);
        } catch (error) {
            message.error(error instanceof Error ? error.message : "工作流详情加载失败");
        }
    }, [message]);

    const cleanupPendingCovers = useCallback(async (exceptResourceId = "") => {
        const ids = pendingCoverIds.current.filter((id) => id !== exceptResourceId);
        pendingCoverIds.current = pendingCoverIds.current.filter((id) => id === exceptResourceId);
        await Promise.all(ids.map((id) => discardAdminCanvasWorkflowCover(id).catch(() => undefined)));
    }, []);

    const closeDetail = useCallback(() => {
        setDetailOpen(false);
        setCoverCropOpen(false);
        if (coverCropSource) URL.revokeObjectURL(coverCropSource);
        setCoverCropSource("");
        if (localCoverPreview.current) URL.revokeObjectURL(localCoverPreview.current);
        localCoverPreview.current = "";
        setCoverPreview("");
        void cleanupPendingCovers();
    }, [cleanupPendingCovers, coverCropSource]);

    const chooseCover = useCallback((file?: File) => {
        if (!file) return;
        const allowed = ["image/png", "image/jpeg", "image/webp"];
        if (!allowed.includes(file.type) || file.size <= 0 || file.size > 10 * 1024 * 1024) {
            message.error("封面仅支持 PNG、JPEG 或 WebP，最大 10MB");
            return;
        }
        if (coverCropSource) URL.revokeObjectURL(coverCropSource);
        setCoverCropSource(URL.createObjectURL(file));
        setCoverCropOpen(true);
    }, [coverCropSource, message]);

    const confirmCoverCrop = useCallback(async (crop: ImageCropRect) => {
        if (!coverCropSource) return;
        setCoverUploading(true);
        try {
            const cropped = await cropWorkflowCover(coverCropSource, crop);
            const uploaded = await uploadAdminCanvasWorkflowCover(cropped);
            pendingCoverIds.current.push(uploaded.resource.id);
            if (localCoverPreview.current) URL.revokeObjectURL(localCoverPreview.current);
            localCoverPreview.current = URL.createObjectURL(cropped);
            setCoverPreview(localCoverPreview.current);
            setCoverResourceId(uploaded.resource.id);
            setCoverCropOpen(false);
            URL.revokeObjectURL(coverCropSource);
            setCoverCropSource("");
            message.success("封面已上传到草稿，保存并发布后前台可见");
        } catch (error) {
            message.error(error instanceof Error ? error.message : "封面上传失败");
        } finally {
            setCoverUploading(false);
        }
    }, [coverCropSource, message]);

    const saveCatalogMetadata = useCallback(async () => {
        if (!detail) return;
        const previousCoverResourceId = detail.draftCoverResourceId || "";
        try {
            const result = await updateCanvasWorkflowMeta(detail.id, detail.revision, {
                description: catalogDescription,
                usageGuide,
                coverResourceId,
            });
            setDetail(result.workflow);
            setCatalogDescription(result.workflow.draftDescription || "");
            setUsageGuide(result.workflow.draftUsageGuide || "");
            setCoverResourceId(result.workflow.draftCoverResourceId || "");
            setCoverPreview(result.workflow.draftCoverUrl || "");
            await cleanupPendingCovers(coverResourceId);
            if (previousCoverResourceId && previousCoverResourceId !== coverResourceId) {
                await discardAdminCanvasWorkflowCover(previousCoverResourceId).catch(() => undefined);
            }
            if (localCoverPreview.current) URL.revokeObjectURL(localCoverPreview.current);
            localCoverPreview.current = "";
            message.success("目录资料已保存到草稿；发布版本后前台生效");
            await load();
        } catch (error) {
            message.error(error instanceof Error ? error.message : "目录资料保存失败");
        }
    }, [catalogDescription, cleanupPendingCovers, coverResourceId, detail, load, message, usageGuide]);

    // 在线编辑：把草稿物化成临时画布，跳到原生画布编辑器。
    // 保存动作在画布页完成（识别 ?workflowTemplate= 参数），这里只负责打开。
    const startEditing = useCallback(async (record: AdminCanvasWorkflowSummary) => {
        try {
            const result = await openCanvasWorkflowEditSession(record.id);
            const session = result.session;
            navigate(`/canvas/${session.canvasId}?workflowTemplate=${encodeURIComponent(record.id)}&workflowTemplateRevision=${record.revision}`);
        } catch (error) {
            message.error(error instanceof Error ? error.message : "打开编辑器失败");
        }
    }, [message, navigate]);

    const refreshDetail = useCallback(async (id: string) => {
        const result = await getAdminCanvasWorkflow(id);
        setDetail(result.workflow);
        await load();
    }, [load]);

    const openCreate = useCallback(async () => {
        setCreateOpen(true);
        try {
            const page = await listRemoteCanvasProjectsPage({ page: 1, pageSize: 100 });
            setCanvases((page.projects || []).map((item) => ({ id: item.id, title: item.title || "未命名画布" })));
        } catch {
            setCanvases([]);
        }
    }, []);

    const submitCreate = useCallback(async () => {
        try {
            const values = await createForm.validateFields();
            const result = await createCanvasWorkflowFromCanvas({
                canvasId: values.canvasId,
                slug: values.slug,
                name: values.name,
                description: values.description,
                category: values.category,
                sortOrder: values.sortOrder ?? 0,
                enabled: values.enabled ?? true,
            });
            message.success("已从画布创建模板，请检查后发布");
            setCreateOpen(false);
            createForm.resetFields();
            await load();
            setDetail(result.workflow);
            setDetailOpen(true);
        } catch (error) {
            if (error instanceof Error) message.error(error.message);
        }
    }, [createForm, load, message]);

    const toggleEnabled = useCallback(async (record: AdminCanvasWorkflowSummary, enabled: boolean) => {
        try {
            await updateCanvasWorkflowMeta(record.id, record.revision, { enabled });
            message.success(enabled ? "已启用" : "已停用（前台不再显示）");
            await load();
            if (detail?.id === record.id) await refreshDetail(record.id);
        } catch (error) {
            message.error(error instanceof Error ? error.message : "操作失败");
        }
    }, [detail?.id, load, message, refreshDetail]);

    const submitPublish = useCallback(async () => {
        if (!detail) return;
        try {
            const values = await publishForm.validateFields();
            await publishCanvasWorkflow(detail.id, detail.revision, values.name, values.notes);
            pendingCoverIds.current = pendingCoverIds.current.filter((id) => id !== coverResourceId);
            message.success("已发布新版本");
            publishForm.resetFields();
            await refreshDetail(detail.id);
        } catch (error) {
            if (error instanceof Error) message.error(error.message);
        }
    }, [coverResourceId, detail, message, publishForm, refreshDetail]);

    const doRollback = useCallback(async (version: AdminCanvasWorkflowVersion) => {
        if (!detail) return;
        try {
            await rollbackCanvasWorkflow(detail.id, version.id, detail.revision);
            message.success(`已把 v${version.version} 载入草稿，确认后再发布`);
            await refreshDetail(detail.id);
        } catch (error) {
            message.error(error instanceof Error ? error.message : "回滚失败");
        }
    }, [detail, message, refreshDetail]);

    const doDelete = useCallback(async (record: AdminCanvasWorkflowSummary) => {
        try {
            await deleteCanvasWorkflow(record.id);
            message.success("已删除");
            if (detail?.id === record.id) setDetailOpen(false);
            await load();
        } catch (error) {
            message.error(error instanceof Error ? error.message : "删除失败");
        }
    }, [detail?.id, load, message]);

    const columns = useMemo<ColumnsType<AdminCanvasWorkflowSummary>>(() => [
        {
            title: "模板",
            dataIndex: "name",
            render: (_, record) => (
                <div className="flex flex-col gap-0.5">
                    <div className="flex items-center gap-2">
                        <span className="font-medium">{record.name}</span>
                        {record.builtin ? <Tag color="blue">内置</Tag> : null}
                        {record.hasUnpublishedDraft ? <Tag color="orange">有未发布改动</Tag> : null}
                    </div>
                    <span className="text-xs text-gray-500">{record.slug}</span>
                </div>
            ),
        },
        { title: "分类", dataIndex: "category", width: 110 },
        {
            title: "版本",
            dataIndex: "version",
            width: 90,
            render: (value: number, record) => (
                <Tooltip title={`线上 ${record.nodeCount} 节点 / 草稿 ${record.draftNodeCount} 节点`}>
                    <span>{record.published ? `v${value}` : "未发布"}</span>
                </Tooltip>
            ),
        },
        {
            title: "导入量",
            width: 150,
            render: (_, record) => (
                <span className="text-xs text-gray-600">
                    总 {record.importTotal} · 7天 {record.importLast7Days} · 30天 {record.importLast30Days}
                </span>
            ),
        },
        {
            title: "启用",
            width: 90,
            render: (_, record) => (
                <Switch size="small" checked={record.enabled} onChange={(checked) => void toggleEnabled(record, checked)} />
            ),
        },
        {
            title: "操作",
            width: 220,
            render: (_, record) => (
                <Space size={4}>
                    <Button size="small" type="primary" ghost icon={<Pencil className="size-3.5" />} onClick={() => void startEditing(record)}>编辑</Button>
                    <Button size="small" icon={<Eye className="size-3.5" />} onClick={() => void openDetail(record.id)}>查看</Button>
                    <Popconfirm
                        title="确认删除该模板？"
                        description="已导入到用户画布的副本不受影响。"
                        onConfirm={() => void doDelete(record)}
                        disabled={record.builtin}
                    >
                        <Button size="small" danger icon={<Trash2 className="size-3.5" />} disabled={record.builtin}>删除</Button>
                    </Popconfirm>
                </Space>
            ),
        },
    ], [doDelete, openDetail, startEditing, toggleEnabled]);

    return (
        <AdminPageFrame
            title="工作流模板"
            description="维护前台「工作流」板块的模板：从画布创建、草稿编辑、发布版本、启停与导入统计。"
            actions={
                <Space>
                    <Button icon={<RefreshCw className="size-4" />} onClick={() => void load()} loading={loading}>刷新</Button>
                    <Button type="primary" icon={<Plus className="size-4" />} onClick={() => void openCreate()}>从画布创建</Button>
                </Space>
            }
        >
            <Table
                rowKey="id"
                size="small"
                loading={loading}
                dataSource={items}
                columns={columns}
                pagination={false}
                locale={{ emptyText: <EmptyState title="还没有工作流模板" description="点击右上角「从画布创建」，把已搭好的画布存成模板。" /> }}
            />

            <Modal
                title="从画布创建模板"
                open={createOpen}
                onCancel={() => setCreateOpen(false)}
                onOk={() => void submitCreate()}
                okText="创建"
                width={560}
            >
                <Form form={createForm} layout="vertical" initialValues={{ enabled: true, sortOrder: 0 }}>
                    <Form.Item name="canvasId" label="选择画布" rules={[{ required: true, message: "请选择画布" }]}>
                        <Select
                            showSearch
                            optionFilterProp="label"
                            placeholder="选择一张已搭建好的画布"
                            options={canvases.map((item) => ({ value: item.id, label: item.title }))}
                        />
                    </Form.Item>
                    <Form.Item name="slug" label="模板标识" rules={[{ required: true, message: "请填写标识" }, { pattern: /^[a-z0-9]+(-[a-z0-9]+)*$/, message: "只能用小写字母、数字和连字符" }]}>
                        <Input placeholder="ecommerce-product-detail-suite" />
                    </Form.Item>
                    <Form.Item name="name" label="模板名称" rules={[{ required: true, message: "请填写名称" }]}>
                        <Input placeholder="电商主图＋详情页套装" />
                    </Form.Item>
                    <Form.Item name="description" label="描述">
                        <Input.TextArea rows={2} />
                    </Form.Item>
                    <Form.Item name="category" label="分类">
                        <Input placeholder="电商设计" />
                    </Form.Item>
                    <Form.Item name="sortOrder" label="排序">
                        <InputNumber min={0} />
                    </Form.Item>
                    <Form.Item name="enabled" label="创建后立即启用" valuePropName="checked">
                        <Switch />
                    </Form.Item>
                </Form>
            </Modal>

            <Drawer
                title={detail ? detail.name : "工作流详情"}
                open={detailOpen}
                onClose={closeDetail}
                width={720}
                extra={detail ? (
                    <Space>
                        <Button icon={<Pencil className="size-4" />} onClick={() => void startEditing(detail)}>
                            在画布中编辑
                        </Button>
                        <Button icon={<Rocket className="size-4" />} type="primary" onClick={() => publishForm.scrollToField?.("name")}>
                            去发布
                        </Button>
                    </Space>
                ) : null}
            >
                {detail ? (
                    <div className="flex flex-col gap-5">
                        <section className="rounded-lg border border-gray-200 p-4">
                            <h3 className="mb-3 text-sm font-semibold">前台目录资料</h3>
                            <div className="grid gap-4">
                                <div>
                                    <div className="mb-1 flex items-center justify-between gap-3">
                                        <label htmlFor="workflow-catalog-description" className="text-xs font-medium">卡片简介</label>
                                        <span className="text-[11px] text-gray-500">{catalogDescription.length}/500</span>
                                    </div>
                                    <Input.TextArea id="workflow-catalog-description" maxLength={500} rows={2} value={catalogDescription} onChange={(event) => setCatalogDescription(event.target.value)} placeholder="简要说明这个工作流的用途和主要结果" />
                                </div>
                                <div>
                                    <div className="mb-1 text-xs font-medium">详情使用指南（Markdown）</div>
                                    <Input.TextArea rows={7} maxLength={20000} value={usageGuide} onChange={(event) => setUsageGuide(event.target.value)} placeholder={"## 适用场景\n说明工作流适合解决什么问题。\n\n## 所需素材\n列出用户需要准备的图片、视频或文案。\n\n## 操作步骤\n1. 替换输入素材\n2. 检查提示词与模型\n3. 逐节点运行并检查结果\n\n## 注意事项\n说明模型能力、费用或内容限制。"} />
                                    <p className="mt-1 text-[11px] text-gray-500">使用指南随发布版本保存；建议写明用途、输入素材、操作步骤、输出和注意事项。</p>
                                </div>
                                <div>
                                    <div className="mb-2 flex items-center justify-between gap-3">
                                        <div>
                                            <div className="text-xs font-medium">目录封面</div>
                                            <p className="mt-1 text-[11px] text-gray-500">推荐 1200×675 px（16:9），支持 PNG/JPEG/WebP，最大 10MB；上传后可调整裁切区域。</p>
                                        </div>
                                        <Space>
                                            <Button icon={<Upload className="size-4" />} loading={coverUploading} onClick={() => coverInputRef.current?.click()}>上传封面</Button>
                                            {coverResourceId ? <Button danger onClick={() => { setCoverResourceId(""); setCoverPreview(""); }}>移除</Button> : null}
                                        </Space>
                                    </div>
                                    <input ref={coverInputRef} type="file" accept="image/png,image/jpeg,image/webp" className="hidden" onChange={(event) => { chooseCover(event.target.files?.[0]); event.currentTarget.value = ""; }} />
                                    {coverPreview ? (
                                        <div className="overflow-hidden rounded-lg border border-gray-200">
                                            <img src={coverPreview.startsWith("blob:") ? coverPreview : adminCanvasWorkflowDraftCoverURL(detail.id)} alt="工作流封面草稿预览" className="aspect-video w-full object-cover" />
                                        </div>
                                    ) : (
                                        <div className="grid aspect-video place-items-center rounded-lg border border-dashed border-gray-300 bg-gray-50 text-xs text-gray-500">
                                            <span className="inline-flex items-center gap-2"><ImagePlus className="size-4" />使用节点素材作为默认封面</span>
                                        </div>
                                    )}
                                </div>
                                <div className="flex items-center justify-between gap-3">
                                    <span className="text-[11px] text-gray-500">保存后只更新草稿；发布新版本后前台同步更新。</span>
                                    <Button type="primary" onClick={() => void saveCatalogMetadata()}>保存目录资料</Button>
                                </div>
                            </div>
                        </section>
                        <section>
                            <h3 className="mb-2 text-sm font-semibold">基本信息</h3>
                            <div className="grid grid-cols-2 gap-3 text-xs text-gray-600">
                                <div>标识：{detail.slug}</div>
                                <div>分类：{detail.category || "未分类"}</div>
                                <div>状态：{detail.enabled ? "已启用" : "已停用"}</div>
                                <div>发布：{detail.published ? `v${detail.version}` : "未发布"}</div>
                                <div>线上节点：{detail.nodeCount}</div>
                                <div>草稿节点：{detail.draftNodeCount}</div>
                                <div>导入总量：{detail.importTotal}</div>
                                <div>近 7 天：{detail.importLast7Days}</div>
                            </div>
                            {detail.hasUnpublishedDraft ? (
                                <div className="mt-3 flex items-center gap-2 rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-700">
                                    <AlertTriangle className="size-3.5" />
                                    草稿与线上版本不同，发布后才会对用户生效。
                                </div>
                            ) : null}
                        </section>

                        <section>
                            <h3 className="mb-2 flex items-center gap-2 text-sm font-semibold">
                                <History className="size-4" />版本历史
                            </h3>
                            <Table
                                rowKey="id"
                                size="small"
                                pagination={false}
                                dataSource={detail.history}
                                columns={[
                                    { title: "版本", dataIndex: "version", width: 70, render: (v: number) => `v${v}` },
                                    { title: "名称", dataIndex: "name", render: (v: string) => v || "—" },
                                    { title: "节点", dataIndex: "nodeCount", width: 70 },
                                    { title: "时间", dataIndex: "createdAt", width: 160, render: (v: string) => new Date(v).toLocaleString("zh-CN") },
                                    {
                                        title: "操作",
                                        width: 150,
                                        render: (_, version) => (
                                            <Space size={4}>
                                                {version.isLatest ? <Tag color="green">当前线上</Tag> : null}
                                                <Button
                                                    size="small"
                                                    icon={<RotateCcw className="size-3.5" />}
                                                    onClick={() => void doRollback(version)}
                                                >
                                                    载入草稿
                                                </Button>
                                            </Space>
                                        ),
                                    },
                                ]}
                            />
                        </section>

                        <section>
                            <h3 className="mb-2 flex items-center gap-2 text-sm font-semibold">
                                <Rocket className="size-4" />发布新版本
                            </h3>
                            {/* 发布表单放在抽屉内，不用 Modal 叠加：antd 的 Drawer 与 Modal
                                焦点陷阱会互相递归，触发 Maximum call stack size exceeded。 */}
                            <Form form={publishForm} layout="vertical" className="rounded-lg border border-gray-100 bg-gray-50/60 p-3">
                                <Form.Item name="name" label="版本名称（可选）" className="!mb-2">
                                    <Input placeholder="如：新增包装图节点" />
                                </Form.Item>
                                <Form.Item name="notes" label="发布说明（可选）" className="!mb-2">
                                    <Input.TextArea rows={2} placeholder="这次改了什么，方便以后回看" />
                                </Form.Item>
                                    <Button type="primary" icon={<Rocket className="size-4" />} onClick={() => void submitPublish()}>
                                    发布为 v{detail.version + 1}
                                </Button>
                            </Form>
                        </section>

                        <section>
                            <h3 className="mb-2 flex items-center gap-2 text-sm font-semibold">
                                <ArrowUpCircle className="size-4" />草稿预览
                            </h3>
                            <pre className="max-h-72 overflow-auto rounded-lg bg-gray-50 p-3 text-[11px] leading-5 text-gray-600">
                                {JSON.stringify(detail.draft, null, 2)}
                            </pre>
                        </section>
                    </div>
                ) : null}
            </Drawer>
            <ImageCropDialog
                dataUrl={coverCropSource}
                open={coverCropOpen}
                onClose={() => {
                    setCoverCropOpen(false);
                    if (coverCropSource) URL.revokeObjectURL(coverCropSource);
                    setCoverCropSource("");
                }}
                onConfirm={(crop) => void confirmCoverCrop(crop)}
                fixedAspect={16 / 9}
                title="裁切工作流封面 · 1200×675"
                confirmLoading={coverUploading}
            />


        </AdminPageFrame>
    );
}
