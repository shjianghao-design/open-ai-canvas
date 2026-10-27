package handler

import (
	"net/http"
	"time"

	"yingce/backend/internal/service"

	"github.com/gin-gonic/gin"
)

// RegisterCanvasWorkflowAdminRoutes 是工作流模板的后台维护入口。
// 所有写操作都要求管理员身份，并由 service 层写审计。
func RegisterCanvasWorkflowAdminRoutes(r *gin.RouterGroup, svc *service.Service) {
	r.POST("/admin/canvas-workflow-covers", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		policy, available := loadRuntimePolicy(c, svc)
		if !available || !enforceRateLimit(c, "admin-workflow-cover-upload:"+user.ID, policy.Request.ResourceUploadPerMinute, time.Minute) {
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.CanvasWorkflowCoverMaxBytes+(1<<20))
		file, err := c.FormFile("file")
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		resource, err := svc.UploadCanvasWorkflowCover(user, file)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"resource": resource})
	})
	r.DELETE("/admin/canvas-workflow-covers/:resourceId", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		if err := svc.DiscardCanvasWorkflowCover(user, c.Param("resourceId")); err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"ok": true})
	})
	r.GET("/admin/canvas-workflows/:id/draft-cover", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		delivery, err := svc.PrepareAdminCanvasWorkflowDraftCover(user, c.Param("id"), resourceAccessOptions(c), c.GetHeader("Range"))
		if err != nil {
			failService(c, err)
			return
		}
		serveResourceDelivery(c, delivery, "private, no-cache", "")
	})
	r.GET("/admin/canvas-workflows", func(c *gin.Context) {
		actor, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		items, err := svc.AdminCanvasWorkflows(actor)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"workflows": items})
	})

	r.GET("/admin/canvas-workflows/:id", func(c *gin.Context) {
		actor, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		detail, err := svc.AdminCanvasWorkflow(actor, c.Param("id"))
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"workflow": detail})
	})

	// 从画布创建模板：方案 A 的入口，管理员在真实画布上搭好再一键存为模板。
	r.POST("/admin/canvas-workflows/from-canvas", func(c *gin.Context) {
		actor, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
		var req service.AdminCanvasWorkflowCreateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		detail, err := svc.CreateCanvasWorkflowFromCanvas(actor, req)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"workflow": detail})
	})

	// 在线编辑：把模板草稿物化成临时画布，交给原生画布编辑器打开。
	r.POST("/admin/canvas-workflows/:id/edit-session", func(c *gin.Context) {
		actor, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		session, err := svc.OpenCanvasWorkflowEditSession(actor, c.Param("id"))
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"session": session})
	})

	// 在线编辑保存：把画布内容写回模板草稿（不发布）。
	r.POST("/admin/canvas-workflows/:id/draft/from-canvas", func(c *gin.Context) {
		actor, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
		var req struct {
			CanvasID         string `json:"canvasId"`
			ExpectedRevision int64  `json:"expectedRevision"`
			DeleteCanvas     bool   `json:"deleteCanvas"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		detail, err := svc.SaveCanvasWorkflowDraftFromCanvas(actor, c.Param("id"), req.CanvasID, req.ExpectedRevision, req.DeleteCanvas)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"workflow": detail})
	})

	r.PUT("/admin/canvas-workflows/:id/draft", func(c *gin.Context) {
		actor, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
		var req service.AdminCanvasWorkflowDraftRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		detail, err := svc.SaveCanvasWorkflowDraft(actor, c.Param("id"), req)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"workflow": detail})
	})

	r.PATCH("/admin/canvas-workflows/:id", func(c *gin.Context) {
		actor, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
		var req service.AdminCanvasWorkflowMetaRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		detail, err := svc.UpdateCanvasWorkflowMeta(actor, c.Param("id"), req)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"workflow": detail})
	})

	r.POST("/admin/canvas-workflows/:id/publish", func(c *gin.Context) {
		actor, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
		var req service.AdminCanvasWorkflowPublishRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		detail, err := svc.PublishCanvasWorkflow(actor, c.Param("id"), req)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"workflow": detail})
	})

	r.POST("/admin/canvas-workflows/:id/rollback", func(c *gin.Context) {
		actor, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var req struct {
			VersionID        string `json:"versionId"`
			ExpectedRevision int64  `json:"expectedRevision"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		detail, err := svc.RollbackCanvasWorkflow(actor, c.Param("id"), req.VersionID, req.ExpectedRevision)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"workflow": detail})
	})

	r.DELETE("/admin/canvas-workflows/:id", func(c *gin.Context) {
		actor, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		if err := svc.DeleteCanvasWorkflow(actor, c.Param("id")); err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"ok": true})
	})

	// 版本更新提示：只提示，不强制用户重新导入。
	r.POST("/canvas-workflows/update-hints", func(c *gin.Context) {
		actor, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var req struct {
			CanvasIDs []string `json:"canvasIds"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		hints, err := svc.CanvasWorkflowUpdateHints(actor.ID, req.CanvasIDs)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"hints": hints})
	})
}
