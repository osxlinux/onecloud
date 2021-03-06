// Copyright 2019 Yunion
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package db

import (
	"context"

	"yunion.io/x/jsonutils"
	"yunion.io/x/pkg/errors"
	"yunion.io/x/pkg/utils"
	"yunion.io/x/sqlchemy"

	"yunion.io/x/onecloud/pkg/apis"
	api "yunion.io/x/onecloud/pkg/apis/compute"
	"yunion.io/x/onecloud/pkg/httperrors"
	"yunion.io/x/onecloud/pkg/mcclient"
	"yunion.io/x/onecloud/pkg/util/rbacutils"
	"yunion.io/x/onecloud/pkg/util/stringutils2"
)

type SScopedResourceBaseManager struct {
	SProjectizedResourceBaseManager
}

type SScopedResourceBase struct {
	SProjectizedResourceBase
	// DomainId  string `width:"64" charset:"ascii" nullable:"true" index:"true" list:"user"`
	// ProjectId string `name:"tenant_id" width:"128" charset:"ascii" nullable:"true" index:"true" list:"user"`
}

func (m *SScopedResourceBaseManager) FilterByOwner(q *sqlchemy.SQuery, userCred mcclient.IIdentityProvider, scope rbacutils.TRbacScope) *sqlchemy.SQuery {
	if userCred == nil {
		return q
	}
	switch scope {
	case rbacutils.ScopeDomain:
		q = q.Filter(sqlchemy.OR(
			sqlchemy.Equals(q.Field("domain_id"), userCred.GetProjectDomainId()),
			sqlchemy.IsNullOrEmpty(q.Field("domain_id")),
		))
	case rbacutils.ScopeProject:
		q = q.Filter(sqlchemy.OR(
			sqlchemy.Equals(q.Field("tenant_id"), userCred.GetProjectId()),
			sqlchemy.IsNullOrEmpty(q.Field("tenant_id")),
		))
	}
	return q
}

func (m *SScopedResourceBaseManager) ValidateCreateData(man IScopedResourceManager, ctx context.Context, userCred mcclient.TokenCredential, ownerId mcclient.IIdentityProvider, query jsonutils.JSONObject, input api.ScopedResourceCreateInput) (api.ScopedResourceCreateInput, error) {
	if input.Scope == "" {
		input.Scope = string(rbacutils.ScopeSystem)
	}
	if !utils.IsInStringArray(input.Scope, []string{
		string(rbacutils.ScopeSystem),
		string(rbacutils.ScopeDomain),
		string(rbacutils.ScopeProject)}) {
		return input, httperrors.NewInputParameterError("invalid scope %s", input.Scope)
	}
	var allowCreate bool
	switch rbacutils.TRbacScope(input.Scope) {
	case rbacutils.ScopeSystem:
		allowCreate = IsAdminAllowCreate(userCred, man)
	case rbacutils.ScopeDomain:
		allowCreate = IsDomainAllowCreate(userCred, man)
	case rbacutils.ScopeProject:
		allowCreate = IsProjectAllowCreate(userCred, man)
	}
	if !allowCreate {
		return input, httperrors.NewForbiddenError("not allow create %s in scope %s", man.ResourceScope(), input.Scope)
	}
	return input, nil
}

func getScopedResourceScope(domainId, projectId string) rbacutils.TRbacScope {
	if domainId == "" && projectId == "" {
		return rbacutils.ScopeSystem
	}
	if domainId != "" && projectId == "" {
		return rbacutils.ScopeDomain
	}
	if domainId != "" && projectId != "" {
		return rbacutils.ScopeProject
	}
	return rbacutils.ScopeNone
}

func (s *SScopedResourceBase) GetResourceScope() rbacutils.TRbacScope {
	return getScopedResourceScope(s.DomainId, s.ProjectId)
}

func (s *SScopedResourceBase) GetDomainId() string {
	return s.DomainId
}

func (s *SScopedResourceBase) GetProjectId() string {
	return s.ProjectId
}

func (s *SScopedResourceBase) SetResourceScope(domainId, projectId string) error {
	s.DomainId = domainId
	s.ProjectId = projectId
	return nil
}

func (s *SScopedResourceBase) CustomizeCreate(ctx context.Context, userCred mcclient.TokenCredential, ownerId mcclient.IIdentityProvider, query jsonutils.JSONObject, data jsonutils.JSONObject) error {
	scope, _ := data.GetString("scope")
	switch rbacutils.TRbacScope(scope) {
	case rbacutils.ScopeDomain:
		s.DomainId = ownerId.GetDomainId()
	case rbacutils.ScopeProject:
		s.DomainId = ownerId.GetDomainId()
		s.ProjectId = ownerId.GetProjectId()
	}
	return nil
}

func (s *SScopedResourceBase) GetMoreColumns(extra *jsonutils.JSONDict) *jsonutils.JSONDict {
	if s.ProjectId != "" {
		extra.Add(jsonutils.NewString(s.ProjectId), "project_id")
	}
	return extra
}

type IScopedResourceModel interface {
	IModel

	GetDomainId() string
	GetProjectId() string
	SetResourceScope(domainId, projectId string) error
}

func (m *SScopedResourceBaseManager) PerformSetScope(
	ctx context.Context,
	obj IScopedResourceModel,
	userCred mcclient.TokenCredential,
	data jsonutils.JSONObject,
) (jsonutils.JSONObject, error) {
	domainId := jsonutils.GetAnyString(data, []string{"domain_id", "domain", "project_domain_id", "project_domain"})
	projectId := jsonutils.GetAnyString(data, []string{"project_id", "project"})
	if projectId != "" {
		project, err := TenantCacheManager.FetchTenantByIdOrName(ctx, projectId)
		if err != nil {
			return nil, err
		}
		projectId = project.GetId()
		domainId = project.GetDomainId()
	}
	if domainId != "" {
		domain, err := TenantCacheManager.FetchDomainByIdOrName(ctx, domainId)
		if err != nil {
			return nil, err
		}
		domainId = domain.GetId()
	}
	scopeToSet := getScopedResourceScope(domainId, projectId)
	var err error
	switch scopeToSet {
	case rbacutils.ScopeSystem:
		err = m.SetScopedResourceToSystem(obj, userCred)
	case rbacutils.ScopeDomain:
		err = m.SetScopedResourceToDomain(obj, userCred, domainId)
	case rbacutils.ScopeProject:
		err = m.SetScopedResourceToProject(obj, userCred, projectId)
	}
	return nil, err
}

func setScopedResourceIds(model IScopedResourceModel, userCred mcclient.TokenCredential, domainId, projectId string) error {
	diff, err := Update(model, func() error {
		model.SetResourceScope(domainId, projectId)
		return nil
	})
	if err == nil {
		OpsLog.LogEvent(model, ACT_UPDATE, diff, userCred)
	}
	return err
}

func (m *SScopedResourceBaseManager) SetScopedResourceToSystem(model IScopedResourceModel, userCred mcclient.TokenCredential) error {
	if !IsAdminAllowPerform(userCred, model, "set-scope") {
		return httperrors.NewForbiddenError("Not allow set scope to system")
	}
	if model.GetProjectId() == "" && model.GetDomainId() == "" {
		return nil
	}
	return setScopedResourceIds(model, userCred, "", "")
}

func (m *SScopedResourceBaseManager) SetScopedResourceToDomain(model IScopedResourceModel, userCred mcclient.TokenCredential, domainId string) error {
	if !IsDomainAllowPerform(userCred, model, "set-scope") {
		return httperrors.NewForbiddenError("Not allow set scope to domain %s", domainId)
	}
	if model.GetDomainId() == domainId && model.GetProjectId() == "" {
		return nil
	}
	domain, err := TenantCacheManager.FetchDomainById(context.TODO(), domainId)
	if err != nil {
		return err
	}
	return setScopedResourceIds(model, userCred, domain.GetId(), "")
}

func (m *SScopedResourceBaseManager) SetScopedResourceToProject(model IScopedResourceModel, userCred mcclient.TokenCredential, projectId string) error {
	if !IsProjectAllowPerform(userCred, model, "set-scope") {
		return httperrors.NewForbiddenError("Not allow set scope to project %s", projectId)
	}
	if model.GetProjectId() == projectId {
		return nil
	}
	project, err := TenantCacheManager.FetchTenantById(context.TODO(), projectId)
	if err != nil {
		return err
	}
	return setScopedResourceIds(model, userCred, project.GetDomainId(), projectId)
}

func (m *SScopedResourceBaseManager) ListItemFilter(
	ctx context.Context,
	q *sqlchemy.SQuery,
	userCred mcclient.TokenCredential,
	query apis.ScopedResourceBaseListInput,
) (*sqlchemy.SQuery, error) {
	q, err := m.SProjectizedResourceBaseManager.ListItemFilter(ctx, q, userCred, query.ProjectizedResourceListInput)
	if err != nil {
		return nil, errors.Wrap(err, "SProjectizedResourceBaseManager.ListItemFilter")
	}
	return q, nil
}

func (m *SScopedResourceBaseManager) OrderByExtraFields(
	ctx context.Context,
	q *sqlchemy.SQuery,
	userCred mcclient.TokenCredential,
	query apis.ScopedResourceBaseListInput,
) (*sqlchemy.SQuery, error) {
	q, err := m.SProjectizedResourceBaseManager.OrderByExtraFields(ctx, q, userCred, query.ProjectizedResourceListInput)
	if err != nil {
		return nil, errors.Wrap(err, "SProjectizedResourceBaseManager.OrderByExtraFields")
	}
	return q, nil
}

func (manager *SScopedResourceBaseManager) FetchCustomizeColumns(
	ctx context.Context,
	userCred mcclient.TokenCredential,
	query jsonutils.JSONObject,
	objs []interface{},
	fields stringutils2.SSortedStrings,
	isList bool,
) []apis.ScopedResourceBaseInfo {
	rows := make([]apis.ScopedResourceBaseInfo, len(objs))

	projRows := manager.SProjectizedResourceBaseManager.FetchCustomizeColumns(ctx, userCred, query, objs, fields, isList)

	for i := range rows {
		rows[i] = apis.ScopedResourceBaseInfo{
			ProjectizedResourceInfo: projRows[i],
		}
	}

	return rows
}
