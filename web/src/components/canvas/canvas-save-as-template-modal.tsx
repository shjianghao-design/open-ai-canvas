import { App, Form, Input, InputNumber, Modal, Switch } from "antd";
import { LayoutTemplate } from "lucide-react";
import { useCallback, useState } from "react";

import { createCanvasWorkflowFromCanvas } from "@/services/api/canvas-workflows-admin";

type Props = {
    projectId: string;
    projectTitle: string;
    open: boolean;
    onClose: () => void;
    /** 保存前先把画布落库，保证服务端读到的是最新内容。 */
    beforeSave?: () => unknown;
};

// 「另存为工作流模板」是方案 A 的入口：管理员在真实画布上搭好，一键存成平台模板。
// 模板先进入草稿状态，必须到后台发布后才会出现在前台工作流板块。
export function CanvasSaveAsTemplateModal({ projectId, projectTitle, open, onClose, beforeSave }: Props) {
    const { message } = App.useApp();
    const [form] = Form.useForm();
    const [saving, setSaving] = useState(false);

    const submit = useCallback(async () => {
        try {
            const values = await form.validateFields();
            setSaving(true);
            // 先保存画布，避免服务端拿到旧内容。
            if (beforeSave) await beforeSave();
            const result = await createCanvasWorkflowFromCanvas({
                canvasId: projectId,
                slug: values.slug,
                name: values.name,
                description: values.description,
                category: values.category,
                sortOrder: values.sortOrder ?? 0,
                enabled: values.enabled ?? true,
            });
            message.success(`已保存为模板「${result.workflow.name}」，请到后台发布后前台才可见`);
            form.resetFields();
            onClose();
        } catch (error) {
            if (error instanceof Error) message.error(error.message);
        } finally {
            setSaving(false);
        }
    }, [beforeSave, form, message, onClose, projectId]);

    return (
        <Modal
            title={<span className="flex items-center gap-2"><LayoutTemplate className="size-4" />另存为工作流模板</span>}
            open={open}
            onCancel={onClose}
            onOk={() => void submit()}
            okText="保存为模板"
            confirmLoading={saving}
            width={520}
            destroyOnHidden
        >
            <p className="mb-4 text-xs leading-5 text-gray-500">
                把当前画布保存成平台模板。保存后进入草稿状态，需要在「管理后台 → 工作流模板」发布后，前台用户才能看到。
            </p>
            <Form
                form={form}
                layout="vertical"
                initialValues={{
                    slug: slugify(projectTitle),
                    name: projectTitle,
                    category: "电商设计",
                    sortOrder: 0,
                    enabled: true,
                }}
            >
                <Form.Item name="name" label="模板名称" rules={[{ required: true, message: "请填写模板名称" }]}>
                    <Input placeholder="电商主图＋详情页套装" maxLength={120} />
                </Form.Item>
                <Form.Item
                    name="slug"
                    label="模板标识"
                    rules={[
                        { required: true, message: "请填写标识" },
                        { pattern: /^[a-z0-9]+(-[a-z0-9]+)*$/, message: "只能用小写字母、数字和连字符" },
                    ]}
                    extra="用于 URL，创建后不建议修改"
                >
                    <Input placeholder="ecommerce-product-detail-suite" maxLength={100} />
                </Form.Item>
                <Form.Item name="description" label="描述">
                    <Input.TextArea rows={2} maxLength={500} placeholder="这套模板解决什么问题" />
                </Form.Item>
                <Form.Item name="category" label="分类">
                    <Input placeholder="电商设计" maxLength={64} />
                </Form.Item>
                <Form.Item name="sortOrder" label="排序">
                    <InputNumber min={0} max={9999} />
                </Form.Item>
                <Form.Item name="enabled" label="创建后启用" valuePropName="checked" extra="停用时前台不可见，但仍可在后台维护">
                    <Switch />
                </Form.Item>
            </Form>
        </Modal>
    );
}

// 从画布标题生成合法 slug：中文等非 ASCII 字符会被过滤，兜底用 canvas-template。
function slugify(value: string): string {
    const ascii = value
        .toLowerCase()
        .replace(/[^a-z0-9]+/g, "-")
        .replace(/^-+|-+$/g, "");
    return ascii.length >= 2 ? ascii.slice(0, 100) : "canvas-template";
}
