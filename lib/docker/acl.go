package docker

import (
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/auth"
	"github.com/gravitational/logrus"
)

type registryACL struct {
}

func newACL(options map[string]interface{}) (auth.AccessController, error) {
	return &registryACL{}, nil
}

func (acl *registryACL) Authorized(ctx context.Context, accessItems ...auth.Access) (context.Context, error) {
	logrus.Infof("=== DEBUG === AUTHORIZING REGISTRY ACCESS")
	return ctx, nil
}

func init() {
	auth.Register("gravity", auth.InitFunc(newACL))
}
