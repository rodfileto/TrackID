from pydantic import BaseModel


class AttachIdentityRequest(BaseModel):
    target_entity_id: str
