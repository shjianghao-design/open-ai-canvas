import { loadRasterImage } from "@/lib/image-raster";
import { centeredAspectSelection } from "@/lib/canvas/image-crop-selection";
import { resolveCropPixels, type ImageCropRect } from "@/lib/canvas/image-operation-plan";

export const WORKFLOW_COVER_WIDTH = 1200;
export const WORKFLOW_COVER_HEIGHT = 675;
export const WORKFLOW_COVER_ASPECT = 16 / 9;

export function planWorkflowCoverCrop(size: { width: number; height: number }, selection?: ImageCropRect) {
    const crop = resolveCropPixels(size, selection ?? centeredAspectSelection(size, WORKFLOW_COVER_ASPECT));
    const ratio = crop.width / crop.height;
    if (Math.abs(ratio - WORKFLOW_COVER_ASPECT) > 0.005) throw new RangeError("封面裁切比例必须为 16:9");
    return { source: crop, output: { width: WORKFLOW_COVER_WIDTH, height: WORKFLOW_COVER_HEIGHT } };
}

export async function cropWorkflowCover(source: string, selection: ImageCropRect) {
    const image = await loadRasterImage(source);
    const canvas = document.createElement("canvas");
    try {
        const plan = planWorkflowCoverCrop({ width: image.naturalWidth, height: image.naturalHeight }, selection);
        canvas.width = plan.output.width;
        canvas.height = plan.output.height;
        const context = canvas.getContext("2d");
        if (!context) throw new Error("浏览器无法创建封面裁切画布");
        context.imageSmoothingEnabled = true;
        context.imageSmoothingQuality = "high";
        context.drawImage(image, plan.source.x, plan.source.y, plan.source.width, plan.source.height, 0, 0, canvas.width, canvas.height);
        const blob = await new Promise<Blob>((resolve, reject) => {
            canvas.toBlob((result) => result && result.size ? resolve(result) : reject(new Error("封面导出失败")), "image/jpeg", 0.9);
        });
        return new File([blob], "workflow-cover.jpg", { type: "image/jpeg" });
    } finally {
        image.src = "";
        canvas.width = 0;
        canvas.height = 0;
    }
}
