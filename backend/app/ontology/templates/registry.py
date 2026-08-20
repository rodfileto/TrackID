"""
Registry of built-in TargetTypeTemplates. User-defined templates are a
later step (storage/API layer not built yet - see docs/DTID_ARCHITECTURE.md
"TargetTypeTemplate & Concept"); this is just the built-in half of that
"built-in + user-created" split for now.
"""
from app.ontology.schemas import TargetTypeTemplate
from app.ontology.templates.bank_robbery import BANK_ROBBERY_TEMPLATE

BUILTIN_TEMPLATES: list[TargetTypeTemplate] = [
    BANK_ROBBERY_TEMPLATE,
]


def get_builtin_template(name: str) -> TargetTypeTemplate | None:
    for template in BUILTIN_TEMPLATES:
        if template.name == name:
            return template
    return None
