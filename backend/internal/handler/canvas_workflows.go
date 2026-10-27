package handler

import (
	"net/http"

	"yingce/backend/internal/service"

	"github.com/gin-gonic/gin"
)

func RegisterCanvasWorkflowRoutes(r *gin.RouterGroup, svc *service.Service) {
	r.GET("/canvas-workflows", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			fail(c, http.StatusUnauthorized, err)
			return
		}
		templates, err := svc.CanvasWorkflowTemplates(user.ID)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"workflows": templates})
	})
	r.GET("/canvas-workflows/:slug", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			fail(c, http.StatusUnauthorized, err)
			return
		}
		template, err := svc.CanvasWorkflowTemplate(user.ID, c.Param("slug"))
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"workflow": template})
	})
	r.GET("/canvas-workflows/:slug/cover", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			fail(c, http.StatusUnauthorized, err)
			return
		}
		delivery, err := svc.PrepareCanvasWorkflowCoverDelivery(user, c.Param("slug"), resourceAccessOptions(c), c.GetHeader("Range"))
		if err != nil {
			failService(c, err)
			return
		}
		serveResourceDelivery(c, delivery, "private, max-age=300", "")
	})
	r.POST("/canvas-workflows/:slug/import", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			fail(c, http.StatusUnauthorized, err)
			return
		}
		project, err := svc.ImportCanvasWorkflowTemplate(user.ID, c.Param("slug"))
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"project": project})
	})
}
