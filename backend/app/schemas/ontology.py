from pydantic import BaseModel


class TemplateSummary(BaseModel):
    """List-view shape for a TargetTypeTemplate - name/description/source
    plus a concept count, without the full concept/relation detail."""

    name: str
    description: str
    source: str
    concept_count: int


class TemplateListResponse(BaseModel):
    items: list[TemplateSummary]
