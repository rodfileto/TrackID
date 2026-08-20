"""
Built-in TargetTypeTemplate: Bank Robbery.

Matches the running example used throughout docs/DTID_ARCHITECTURE.md and
the Paper 1 evaluation design (a synthetic bank-robbery series). This is
the first built-in template - source="builtin", not tied to any specific
investigation - instantiated per-Target via an OF_TYPE edge once a real
Target references it.
"""
from app.ontology.schemas import ConceptDefinition, ConceptRelation, TargetTypeTemplate

BANK_ROBBERY_TEMPLATE = TargetTypeTemplate(
    name="Bank Robbery",
    description=(
        "A crime series in which one or more actors plan and execute the "
        "forcible theft of cash or valuables from a financial institution, "
        "typically followed by an attempt to convert or move the proceeds."
    ),
    source="builtin",
    concepts=[
        ConceptDefinition(
            name="Reconnaissance",
            description=(
                "Pre-operation observation of the target location - casing "
                "visits, identifying staff schedules, security posture, and "
                "camera placement. Evidence here typically precedes the "
                "other concepts chronologically."
            ),
        ),
        ConceptDefinition(
            name="Logistics",
            description=(
                "Acquisition and coordination of what the operation needs: "
                "weapons, disguises, vehicles, communication devices, and "
                "personnel roles. Supports both the robbery itself and the "
                "escape."
            ),
        ),
        ConceptDefinition(
            name="Violent Actions",
            description=(
                "The robbery itself - coercion, threats, or physical "
                "violence used against staff or bystanders during the "
                "incident."
            ),
        ),
        ConceptDefinition(
            name="Getaway",
            description=(
                "Escape from the scene - vehicle or route used to leave "
                "the location immediately after the robbery."
            ),
        ),
        ConceptDefinition(
            name="Money Laundering",
            description=(
                "Conversion or movement of stolen proceeds to obscure "
                "their origin - typically the last concept evidenced "
                "chronologically, sometimes well after the robbery itself."
            ),
        ),
    ],
    relations=[
        ConceptRelation(source="Reconnaissance", target="Logistics"),
        ConceptRelation(source="Logistics", target="Violent Actions"),
        ConceptRelation(source="Logistics", target="Getaway"),
        ConceptRelation(source="Violent Actions", target="Getaway"),
        ConceptRelation(source="Getaway", target="Money Laundering"),
    ],
)
