import { apiBaseURL, http } from "@/services/api/request";
import type { RemoteResource } from "@/services/api/resources";

export type AdminCanvasWorkflowSummary = {
    id: string;
    slug: string;
    name: string;
    description: string;
    category: string;
    icon: string;
    sortOrder: number;
    enabled: boolean;
    published: boolean;
    builtin: boolean;
    version: number;
    latestVersionId: string;
    revision: number;
    nodeCount: number;
    draftNodeCount: number;
    hasUnpublishedDraft: boolean;
    importTotal: number;
    importLast7Days: number;
    importLast30Days: number;
    updatedAt: string;
};

export type AdminCanvasWorkflowVersion = {
    id: string;
    version: number;
    name: string;
    notes: string;
    nodeCount: number;
    createdBy: string;
    createdAt: string;
    isLatest: boolean;
};

export type AdminCanvasWorkflowDetail = AdminCanvasWorkflowSummary & {
    draftDescription: string;
    draftUsageGuide: string;
    draftCoverResourceId?: string;
    draftCoverUrl?: string;
    draft: Record<string, unknown>;
    latest?: Record<string, unknown>;
    history: AdminCanvasWorkflowVersion[];
};

export function listAdminCanvasWorkflows() {
    return http.get<{ workflows: AdminCanvasWorkflowSummary[] }>("/admin/canvas-workflows");
}

export function getAdminCanvasWorkflow(id: string) {
    return http.get<{ workflow: AdminCanvasWorkflowDetail }>(`/admin/canvas-workflows/${encodeURIComponent(id)}`);
}

export function createCanvasWorkflowFromCanvas(params: { canvasId: string; slug: string; name: string; description?: string; category?: string; icon?: string; sortOrder?: number; enabled?: boolean }) {
    return http.post<{ workflow: AdminCanvasWorkflowDetail }>("/admin/canvas-workflows/from-canvas", params);
}

export function saveCanvasWorkflowDraft(id: string, expectedRevision: number, payload: Record<string, unknown>) {
    return http.put<{ workflow: AdminCanvasWorkflowDetail }>(`/admin/canvas-workflows/${encodeURIComponent(id)}/draft`, { expectedRevision, payload });
}

export function updateCanvasWorkflowMeta(id: string, expectedRevision: number, patch: Record<string, unknown>) {
    return http.patch<{ workflow: AdminCanvasWorkflowDetail }>(`/admin/canvas-workflows/${encodeURIComponent(id)}`, { expectedRevision, ...patch });
}

export function uploadAdminCanvasWorkflowCover(file: File) {
    const body = new FormData();
    body.append("file", file, file.name);
    return http.post<{ resource: RemoteResource }>("/admin/canvas-workflow-covers", body);
}

export function discardAdminCanvasWorkflowCover(resourceId: string) {
    return http.delete<{ ok: boolean }>(`/admin/canvas-workflow-covers/${encodeURIComponent(resourceId)}`);
}

export function adminCanvasWorkflowDraftCoverURL(id: string) {
    const path = `/api/admin/canvas-workflows/${encodeURIComponent(id)}/draft-cover`;
    const base = String(apiBaseURL).replace(/\/+$/, "");
    return base === "/api" ? path : `${base}${path.slice(4)}`;
}

export function publishCanvasWorkflow(id: string, expectedRevision: number, name?: string, notes?: string) {
    return http.post<{ workflow: AdminCanvasWorkflowDetail }>(`/admin/canvas-workflows/${encodeURIComponent(id)}/publish`, { expectedRevision, name: name ?? "", notes: notes ?? "" });
}

export function rollbackCanvasWorkflow(id: string, versionId: string, expectedRevision: number) {
    return http.post<{ workflow: AdminCanvasWorkflowDetail }>(`/admin/canvas-workflows/${encodeURIComponent(id)}/rollback`, { versionId, expectedRevision });
}

export function deleteCanvasWorkflow(id: string) {
    return http.delete<{ ok: boolean }>(`/admin/canvas-workflows/${encodeURIComponent(id)}`);
}

export type CanvasWorkflowEditSession = {
    canvasId: string;
    templateId: string;
    templateName: string;
};

/** 把模板草稿物化成临时画布，返回可在原生画布编辑器中打开的 canvasId。 */
export function openCanvasWorkflowEditSession(id: string) {
    return http.post<{ session: CanvasWorkflowEditSession }>(`/admin/canvas-workflows/${encodeURIComponent(id)}/edit-session`, {});
}

/** 把画布内容写回模板草稿（不发布）。 */
export function saveCanvasWorkflowDraftFromCanvas(id: string, canvasId: string, expectedRevision: number, deleteCanvas = true) {
    return http.post<{ workflow: AdminCanvasWorkflowDetail }>(`/admin/canvas-workflows/${encodeURIComponent(id)}/draft/from-canvas`, {
        canvasId,
        expectedRevision,
        deleteCanvas,
    });
}
