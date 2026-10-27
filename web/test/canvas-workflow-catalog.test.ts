import { expect, test } from "bun:test";

import { filterCanvasWorkflowCatalog, paginateCanvasWorkflowCatalog } from "../src/lib/canvas-workflow-catalog";
import type { CanvasWorkflowSummary } from "../src/services/api/canvas-workflows";

const workflow = (patch: Partial<CanvasWorkflowSummary> & Pick<CanvasWorkflowSummary, "slug" | "name">): CanvasWorkflowSummary => ({
    slug: patch.slug,
    name: patch.name,
    description: "服装商品流程",
    category: "电商设计",
    version: 1,
    builtin: false,
    nodeCount: 8,
    mediaTypes: ["image"],
    updatedAt: "2026-10-27T00:00:00Z",
    importTotal: 0,
    ...patch,
});

test("catalog search and filters match text, media type, category and popularity", () => {
    const values = [
        workflow({ slug: "new-image", name: "新款服装主图", importTotal: 2 }),
        workflow({ slug: "video", name: "短视频换装", category: "视频制作", mediaTypes: ["video"], importTotal: 30 }),
        workflow({ slug: "old-image", name: "服装详情页", updatedAt: "2026-01-01T00:00:00Z", importTotal: 10 }),
    ];

    expect(filterCanvasWorkflowCatalog(values, { query: "短视频", category: "全部", mediaType: "all", sort: "recent" }).map((item) => item.slug)).toEqual(["video"]);
    expect(filterCanvasWorkflowCatalog(values, { query: "", category: "电商设计", mediaType: "image", sort: "popular" }).map((item) => item.slug)).toEqual(["old-image", "new-image"]);
});

test("catalog pagination uses a fixed 30-row page and clamps stale page indexes", () => {
    const values = Array.from({ length: 61 }, (_, index) => workflow({ slug: `flow-${index}`, name: `流程 ${index}` }));
    expect(paginateCanvasWorkflowCatalog(values, 2)).toMatchObject({ page: 2, pageSize: 30, total: 61, pageCount: 3, items: values.slice(30, 60) });
    expect(paginateCanvasWorkflowCatalog(values, 9)).toMatchObject({ page: 3, pageSize: 30, items: [values[60]] });
    expect(paginateCanvasWorkflowCatalog([], 2)).toMatchObject({ page: 1, pageSize: 30, total: 0, pageCount: 0, items: [] });
});
