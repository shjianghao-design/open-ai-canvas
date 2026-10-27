import { workspaceNavActiveId } from "@/components/layout/workspace-sidebar-nav";
import { expect, test } from "bun:test";

test("工作流列表和只读详情共享导航高亮", () => {
    expect(workspaceNavActiveId("/workflows")).toBe("workflows");
    expect(workspaceNavActiveId("/workflows/ecommerce-product-detail-suite")).toBe("workflows");
});
