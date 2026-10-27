import { App, Button } from "antd";
import { Check, FilePenLine, X } from "lucide-react";
import { useCallback, useState } from "react";
import { useNavigate, useSearchParams } from "react-router";

import { saveCanvasWorkflowDraftFromCanvas } from "@/services/api/canvas-workflows-admin";

type Props = {
    /** 当前画布 ID，用于把内容写回模板草稿。 */
    projectId: string;
    /** 保存前先把画布落库，保证服务端读到最新内容。 */
    onSaved?: () => unknown;
};

// 从后台「工作流模板 → 编辑」进入画布时出现的操作条。
// 只处理「保存回模板草稿」和「返回后台」；发布仍在后台完成，避免误把半成品上线。
export function CanvasTemplateEditBar({ projectId, onSaved }: Props) {
    const { message } = App.useApp();
    const navigate = useNavigate();
    const [searchParams] = useSearchParams();
    const [saving, setSaving] = useState(false);

    const templateId = searchParams.get("workflowTemplate") || "";
    const revisionText = searchParams.get("workflowTemplateRevision") || "";
    if (!templateId) return null;

    const saveBack = useCallback(async () => {
        if (saving) return;
        setSaving(true);
        try {
            if (onSaved) await onSaved();
            const revision = Number(revisionText);
            if (!Number.isSafeInteger(revision)) throw new Error("缺少模板版本号，请回到后台重新打开编辑");
            await saveCanvasWorkflowDraftFromCanvas(templateId, projectId, revision, true);
            message.success("已保存到模板草稿，请回到后台发布");
            navigate("/admin/workflows");
        } catch (error) {
            message.error(error instanceof Error ? error.message : "保存到模板失败");
        } finally {
            setSaving(false);
        }
    }, [message, navigate, onSaved, projectId, revisionText, saving, templateId]);

    return (
        <div
            role="status"
            className="pointer-events-auto mx-4 mb-2 flex flex-wrap items-center gap-2 rounded-xl border border-[#e6d7b8] bg-[#fdf8ee] px-3 py-2 text-[12.5px] text-[#8a6d3b] shadow-[0_2px_10px_rgba(15,23,42,0.06)]"
        >
            <FilePenLine className="size-4 shrink-0" />
            <span className="min-w-0">
                正在编辑工作流模板草稿。修改提示词、布局、节点后点「保存到模板」；发布仍需回到后台完成。
            </span>
            <Button
                size="small"
                type="primary"
                className="ml-auto"
                icon={<Check className="size-3.5" />}
                loading={saving}
                onClick={() => void saveBack()}
            >
                保存到模板
            </Button>
            <Button
                size="small"
                icon={<X className="size-3.5" />}
                onClick={() => navigate("/admin/workflows")}
            >
                返回后台
            </Button>
        </div>
    );
}
