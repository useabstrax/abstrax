package project

import (
	"context"
	"os"
)

// addRollback tracks resources created during project add for cleanup on failure.
type addRollback struct {
	createdDirs []string
	vhostPath   string
	poolPath    string
	acls        []ManagedACL
	webUser     string
	svc         *Service
}

func (r *addRollback) trackDirs(paths []string) {
	r.createdDirs = append(r.createdDirs, paths...)
}

func (r *addRollback) trackVhost(path string) {
	r.vhostPath = path
}

func (r *addRollback) trackPool(path string) {
	r.poolPath = path
}

func (r *addRollback) trackACLs(entries []ManagedACL, webUser string) {
	r.acls = entries
	r.webUser = webUser
}

func (r *addRollback) undo(ctx context.Context) {
	if r.svc == nil {
		return
	}
	acl := newACLManager(r.svc.runner)
	_ = acl.Remove(ctx, r.acls, r.webUser)

	if r.poolPath != "" {
		_ = os.Remove(r.poolPath)
	}

	if r.vhostPath != "" {
		_ = os.Remove(r.vhostPath)
		_, _ = r.svc.runner.Run(ctx, "nginx", "-s", "reload")
	}

	removeCreatedDirs(r.createdDirs)
}
