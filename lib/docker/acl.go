package docker

import (
	"fmt"
	"net/http"

	"github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/auth"

	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"
)

type registryACL struct {
}

func newACL(options map[string]interface{}) (auth.AccessController, error) {
	return &registryACL{}, nil
}

func (acl *registryACL) Authorized(ctx context.Context, accessItems ...auth.Access) (context.Context, error) {
	logrus.Infof("=== DEBUG === AUTHORIZING REGISTRY ACCESS")
	request, err := context.GetRequest(ctx)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	logrus.Infof("=== REQUEST: %#v", request)
	username, password, ok := request.BasicAuth()
	if !ok {
		logrus.Infof("=== NO CREDS")
		return nil, &challenge{
			realm: "basic-realm",
			err:   auth.ErrInvalidCredential,
		}
	}
	logrus.Infof("=== CREDS: %v %v", username, password)
	return auth.WithUser(ctx, auth.UserInfo{
		Name: username,
	}), nil
}

type challenge struct {
	realm string
	err   error
}

var _ auth.Challenge = challenge{}

func (ch challenge) SetHeaders(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", fmt.Sprintf("Basic realm=%q", ch.realm))
}

func (ch challenge) Error() string {
	return fmt.Sprintf("basic authentication challenge for realm %q: %s", ch.realm, ch.err)
}

func init() {
	auth.Register("gravity", auth.InitFunc(newACL))
}
