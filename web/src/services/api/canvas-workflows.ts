import type { CanvasProject } from "@/stores/canvas/use-canvas-store";
import { http } from "@/services/api/request";

export type CanvasWorkflowSummary = {
    slug: string;
    name: string;
    description: string;
    category: string;
    version: number;
    builtin: boolean;
    nodeCount: number;
    mediaTypes?: Array<"image" | "video" | "audio" | "text">;
    coverUrl?: string;
    importTotal?: number;
    updatedAt?: string;
};

export type CanvasWorkflowDetail = CanvasWorkflowSummary & { project: CanvasProject; usageGuide?: string };
export type ImportedCanvasProject = { id: string; title: string; payloadJson: string; revision: number };

export function listCanvasWorkflows() {
    return http.get<{ workflows: CanvasWorkflowSummary[] }>("/canvas-workflows");
}

export function getCanvasWorkflow(slug: string) {
    return http.get<{ workflow: CanvasWorkflowDetail }>(`/canvas-workflows/${encodeURIComponent(slug)}`);
}

export function importCanvasWorkflow(slug: string) {
    return http.post<{ project: ImportedCanvasProject }>(`/canvas-workflows/${encodeURIComponent(slug)}/import`, {});
}

export type CanvasWorkflowUpdateHint = {
    templateSlug: string;
    templateName: string;
    importedVersion: number;
    latestVersion: number;
};

/** 查询这些画布是否有「模板已更新」提示。只提示，不强制重新导入。 */
export function getCanvasWorkflowUpdateHints(canvasIds: string[]) {
    return http.post<{ hints: CanvasWorkflowUpdateHint[] }>("/canvas-workflows/update-hints", { canvasIds });
}
