import { expect, test } from "bun:test";
import { centeredAspectSelection } from "../src/lib/canvas/image-crop-selection";
import { planWorkflowCoverCrop } from "../src/services/workflow-cover-image";

test("workflow cover crops to the fixed 1200 by 675 landscape format", () => {
    const size = { width: 1536, height: 2048 };
    const plan = planWorkflowCoverCrop(size, centeredAspectSelection(size, 16 / 9));
    expect(plan.output).toEqual({ width: 1200, height: 675 });
    expect(plan.source.width / plan.source.height).toBeCloseTo(16 / 9, 2);
});

test("workflow cover crop rejects an incompatible aspect selection", () => {
    expect(() => planWorkflowCoverCrop({ width: 1000, height: 1000 }, { x: 0, y: 0, width: 1, height: 1 })).toThrow("16:9");
});
