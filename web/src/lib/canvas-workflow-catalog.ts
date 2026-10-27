import type { CanvasWorkflowSummary } from "@/services/api/canvas-workflows";

export type CanvasWorkflowMediaFilter = "all" | "image" | "video" | "audio" | "text";
export type CanvasWorkflowSort = "recent" | "popular";
export type CanvasWorkflowCatalogFilters = { query: string; category: string; mediaType: CanvasWorkflowMediaFilter; sort: CanvasWorkflowSort };

export const CANVAS_WORKFLOW_PAGE_SIZE = 30;

export function filterCanvasWorkflowCatalog(items: CanvasWorkflowSummary[], filters: CanvasWorkflowCatalogFilters) {
    const keyword = filters.query.trim().toLocaleLowerCase("zh-CN");
    const filtered = items.filter((item) => {
        if (filters.category !== "全部" && item.category !== filters.category) return false;
        if (filters.mediaType !== "all" && !(item.mediaTypes || []).includes(filters.mediaType)) return false;
        if (!keyword) return true;
        return [item.name, item.description, item.category].join(" ").toLocaleLowerCase("zh-CN").includes(keyword);
    });
    return filtered.sort((left, right) => {
        if (filters.sort === "popular") return (right.importTotal || 0) - (left.importTotal || 0) || dateValue(right.updatedAt) - dateValue(left.updatedAt) || left.name.localeCompare(right.name, "zh-CN");
        return dateValue(right.updatedAt) - dateValue(left.updatedAt) || left.name.localeCompare(right.name, "zh-CN");
    });
}

export function paginateCanvasWorkflowCatalog(items: CanvasWorkflowSummary[], requestedPage: number) {
    const pageCount = Math.ceil(items.length / CANVAS_WORKFLOW_PAGE_SIZE);
    const page = pageCount === 0 ? 1 : Math.min(Math.max(1, Math.floor(requestedPage)), pageCount);
    return {
        page,
        pageSize: CANVAS_WORKFLOW_PAGE_SIZE,
        pageCount,
        total: items.length,
        items: items.slice((page - 1) * CANVAS_WORKFLOW_PAGE_SIZE, page * CANVAS_WORKFLOW_PAGE_SIZE),
    };
}

function dateValue(value: string | undefined) {
    const timestamp = value ? Date.parse(value) : 0;
    return Number.isFinite(timestamp) ? timestamp : 0;
}
