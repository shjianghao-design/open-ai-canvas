import { Button } from "antd";
import { ArrowUpCircle, X } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "react-router";

import { getCanvasWorkflowUpdateHints, type CanvasWorkflowUpdateHint } from "@/services/api/canvas-workflows";

const DISMISS_KEY = "yingce:workflow-update-dismissed";

function readDismissed(): Record<string, number> {
    try {
        const raw = localStorage.getItem(DISMISS_KEY);
        return raw ? (JSON.parse(raw) as Record<string, number>) : {};
    } catch {
        return {};
    }
}

function writeDismissed(value: Record<string, number>) {
    try {
        localStorage.setItem(DISMISS_KEY, JSON.stringify(value));
    } catch {
        // 隐私模式下写不进去，忽略即可，不影响提示功能。
    }
}

type Props = {
    /** 当前画布 ID；为空时不查询。 */
    canvasId: string;
};

// 「模板已更新」提示条：只提示，不强制。
// 用户关闭后按「模板 slug + 最新版本」记录，模板再发新版本会重新提示。
export function CanvasWorkflowUpdateHint({ canvasId }: Props) {
    const navigate = useNavigate();
    const [hint, setHint] = useState<CanvasWorkflowUpdateHint | null>(null);

    useEffect(() => {
        if (!canvasId) {
            setHint(null);
            return;
        }
        let cancelled = false;
        void (async () => {
            try {
                const result = await getCanvasWorkflowUpdateHints([canvasId]);
                if (cancelled) return;
                const next = (result.hints || [])[0] || null;
                if (!next) {
                    setHint(null);
                    return;
                }
                const dismissed = readDismissed();
                if (dismissed[next.templateSlug] === next.latestVersion) {
                    setHint(null);
                    return;
                }
                setHint(next);
            } catch {
                // 提示是附加信息，失败静默，不影响画布编辑。
                if (!cancelled) setHint(null);
            }
        })();
        return () => { cancelled = true; };
    }, [canvasId]);

    const dismiss = useCallback(() => {
        if (!hint) return;
        const dismissed = readDismissed();
        dismissed[hint.templateSlug] = hint.latestVersion;
        writeDismissed(dismissed);
        setHint(null);
    }, [hint]);

    if (!hint) return null;

    return (
        <div
            role="status"
            className="pointer-events-auto mx-4 mb-2 flex flex-wrap items-center gap-2 rounded-xl border border-[#dbe4f5] bg-[#f2f6ff] px-3 py-2 text-[12.5px] text-[#3a5a96] shadow-[0_2px_10px_rgba(15,23,42,0.06)]"
        >
            <ArrowUpCircle className="size-4 shrink-0" />
            <span className="min-w-0">
                模板「{hint.templateName}」已更新到 v{hint.latestVersion}，你当前画布基于 v{hint.importedVersion}。
                你的修改不受影响，可继续使用。
            </span>
            <Button
                size="small"
                type="link"
                className="!px-1"
                onClick={() => navigate(`/workflows/${encodeURIComponent(hint.templateSlug)}`)}
            >
                查看新版
            </Button>
            <button
                type="button"
                onClick={dismiss}
                aria-label="关闭提示"
                className="ml-auto grid size-6 place-items-center rounded-md text-[#7b8aa5] transition hover:bg-[#e3ebfa]"
            >
                <X className="size-3.5" />
            </button>
        </div>
    );
}
