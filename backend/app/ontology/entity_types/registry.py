"""
Registry of built-in EntityTypeDefinitions. User-defined entity types are
a later step (storage/API layer not built yet) - this is just the
built-in half of the "built-in + user-created" split, mirroring
app/ontology/templates/registry.py's pattern for TargetTypeTemplate.
"""
from app.ontology.entity_types.builtin import BUILTIN_ENTITY_TYPES
from app.ontology.schemas import EntityTypeDefinition

__all__ = ["BUILTIN_ENTITY_TYPES", "get_builtin_entity_type"]


def get_builtin_entity_type(name: str) -> EntityTypeDefinition | None:
    for entity_type in BUILTIN_ENTITY_TYPES:
        if entity_type.name == name:
            return entity_type
    return None
