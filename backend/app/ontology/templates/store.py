"""
In-memory store for user-created TargetTypeTemplates.

Explicitly not persistent - resets on backend restart. Same scope
decision as the rest of app/ontology/ so far: real persistence depends
on the Memgraph vs. Postgres storage-boundary question for this data,
which hasn't been settled yet. This just gets the API/UI contract right
ahead of that decision, mirroring the module-level singleton pattern
already used for the ML model in app/core/ml_models.py.
"""
from app.ontology.schemas import TargetTypeTemplate

CREATED_TEMPLATES: list[TargetTypeTemplate] = []


def add_created_template(template: TargetTypeTemplate) -> None:
    CREATED_TEMPLATES.append(template)


def get_created_template(name: str) -> TargetTypeTemplate | None:
    for template in CREATED_TEMPLATES:
        if template.name == name:
            return template
    return None


def replace_created_template(name: str, template: TargetTypeTemplate) -> bool:
    """Replace an existing created template in place. Returns False if
    `name` isn't a created (user-defined) template - builtins never live
    here, so this can't accidentally touch one."""
    for i, existing in enumerate(CREATED_TEMPLATES):
        if existing.name == name:
            CREATED_TEMPLATES[i] = template
            return True
    return False
