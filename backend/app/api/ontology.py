"""
List/get/create TargetTypeTemplates - the Target-Type Conceptual
Framework layer described in docs/DTID_ARCHITECTURE.md. Built-ins come
from app.ontology.templates.registry; created templates are held
in-memory only (app.ontology.templates.store) - see that module's
docstring for why.
"""
from fastapi import APIRouter, HTTPException

from app.ontology.schemas import TargetTypeTemplate
from app.ontology.templates.registry import BUILTIN_TEMPLATES, get_builtin_template
from app.ontology.templates.store import (
    CREATED_TEMPLATES,
    add_created_template,
    get_created_template,
    replace_created_template,
)
from app.schemas.ontology import TemplateListResponse, TemplateSummary

router = APIRouter(prefix="/api/v1/ontology", tags=["ontology"])


@router.get("/templates", response_model=TemplateListResponse)
async def list_templates() -> TemplateListResponse:
    all_templates = BUILTIN_TEMPLATES + CREATED_TEMPLATES
    return TemplateListResponse(
        items=[
            TemplateSummary(
                name=t.name,
                description=t.description,
                source=t.source,
                concept_count=len(t.concepts),
            )
            for t in all_templates
        ]
    )


@router.get("/templates/{name}", response_model=TargetTypeTemplate)
async def get_template(name: str) -> TargetTypeTemplate:
    template = get_builtin_template(name) or get_created_template(name)
    if template is None:
        raise HTTPException(status_code=404, detail=f"Template {name!r} not found")
    return template


@router.post("/templates", response_model=TargetTypeTemplate, status_code=201)
async def create_template(template: TargetTypeTemplate) -> TargetTypeTemplate:
    if get_builtin_template(template.name) or get_created_template(template.name):
        raise HTTPException(
            status_code=409, detail=f"A template named {template.name!r} already exists"
        )

    # source is always "user-defined" for anything created through this
    # endpoint, regardless of what the client sent - "builtin" is reserved
    # for the registry in app.ontology.templates.registry.
    created = template.model_copy(update={"source": "user-defined"})
    add_created_template(created)
    return created


@router.put("/templates/{name}", response_model=TargetTypeTemplate)
async def update_template(name: str, template: TargetTypeTemplate) -> TargetTypeTemplate:
    if get_builtin_template(name) is not None:
        raise HTTPException(
            status_code=403, detail=f"Built-in template {name!r} cannot be modified"
        )
    if get_created_template(name) is None:
        raise HTTPException(status_code=404, detail=f"Template {name!r} not found")

    # Name is the identifier and stays fixed for this call - renaming
    # would need a dedicated rename, not a bundled side effect of an edit.
    # source stays "user-defined" regardless of what the client sent, same
    # rule as create_template.
    updated = template.model_copy(update={"name": name, "source": "user-defined"})
    replace_created_template(name, updated)
    return updated
