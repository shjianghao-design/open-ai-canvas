import { describe, expect, it } from "bun:test";
import { readFileSync } from "node:fs";

const detail = readFileSync(new URL("../src/pages/workflows/detail.tsx", import.meta.url), "utf8");
const index = readFileSync(new URL("../src/pages/workflows/index.tsx", import.meta.url), "utf8");

describe("canvas workflow read-only preview", () => {
    it("renders image, video, audio, text and frame nodes from their existing snapshot", () => {
        expect(detail).toContain("CanvasNodeType.Video");
        expect(detail).toContain("CanvasNodeType.Audio");
        expect(detail).toContain("CanvasNodeType.Frame");
        expect(detail).toContain("metadata?.videoPreview?.content");
        expect(detail).toContain("object-contain");
    });

    it("provides read-only pan and zoom controls without changing the graph layout", () => {
        expect(detail).toContain("onPointerDown");
        expect(detail).toContain("onPointerMove");
        expect(detail).toContain("onWheel");
        expect(detail).toContain("scale(");
        expect(detail).toContain("只读");
        expect(detail).toContain("node.position.x");
        expect(detail).toContain("node.position.y");
    });

    it("shows complete selected-node prompts, source and generation warnings", () => {
        expect(detail).toContain("workflowSource.provider");
        expect(detail).toContain("workflowSource?.promptMasked");
        expect(detail).toContain("workflowSource");
        expect(detail).toContain("workflowGenerationWarning");
        expect(detail).toContain("workflowSample");
        expect(detail).toContain("whitespace-pre-wrap");
    });

    it("derives list covers from real nodes, including non-first media nodes and video posters", () => {
        expect(index).toContain("CanvasNodeType.Image");
        expect(index).toContain("CanvasNodeType.Video");
        expect(index).toContain("metadata?.videoPreview?.content");
        expect(index).toContain("workflow.project.nodes");
        expect(index).not.toContain('["商品", "主图", "详情"]');
    });
});
