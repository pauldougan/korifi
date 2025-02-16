package handlers

import (
	"context"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	RolesPath = "/v3/roles"
)

type RoleName string

const (
	RoleAdmin                      RoleName = "admin"
	RoleAdminReadOnly              RoleName = "admin_read_only"
	RoleGlobalAuditor              RoleName = "global_auditor"
	RoleOrganizationAuditor        RoleName = "organization_auditor"
	RoleOrganizationBillingManager RoleName = "organization_billing_manager"
	RoleOrganizationManager        RoleName = "organization_manager"
	RoleOrganizationUser           RoleName = "organization_user"
	RoleSpaceAuditor               RoleName = "space_auditor"
	RoleSpaceDeveloper             RoleName = "space_developer"
	RoleSpaceManager               RoleName = "space_manager"
	RoleSpaceSupporter             RoleName = "space_supporter"
)

//counterfeiter:generate -o fake -fake-name CFRoleRepository . CFRoleRepository

type CFRoleRepository interface {
	CreateRole(context.Context, authorization.Info, repositories.CreateRoleMessage) (repositories.RoleRecord, error)
}

type RoleHandler struct {
	handlerWrapper   *AuthAwareHandlerFuncWrapper
	apiBaseURL       url.URL
	roleRepo         CFRoleRepository
	decoderValidator *DecoderValidator
}

func NewRoleHandler(apiBaseURL url.URL, roleRepo CFRoleRepository, decoderValidator *DecoderValidator) *RoleHandler {
	return &RoleHandler{
		handlerWrapper:   NewAuthAwareHandlerFuncWrapper(ctrl.Log.WithName("RoleHandler")),
		apiBaseURL:       apiBaseURL,
		roleRepo:         roleRepo,
		decoderValidator: decoderValidator,
	}
}

func (h *RoleHandler) roleCreateHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	var payload payloads.RoleCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, err
	}

	role := payload.ToMessage()
	role.GUID = uuid.NewString()

	record, err := h.roleRepo.CreateRole(ctx, authInfo, role)
	if err != nil {
		logger.Error(err, "Failed to create role", "Role Type", role.Type, "Space", role.Space, "User", role.User)
		return nil, err
	}

	return NewHandlerResponse(http.StatusCreated).WithBody(presenter.ForCreateRole(record, h.apiBaseURL)), nil
}

func (h *RoleHandler) RegisterRoutes(router *mux.Router) {
	router.Path(RolesPath).Methods("POST").HandlerFunc(h.handlerWrapper.Wrap(h.roleCreateHandler))
}
