"""
Built-in EntityTypeDefinitions, per docs/DTID_ARCHITECTURE.md's starter
set: TargetPerson, Vehicle, Place, PhoneNumber, Organization,
FinancialAccount.

default_tau_high/default_tau_low differ by resolution modality, not by
convention - identifiers that either match exactly or don't (a phone
number, an account number) warrant a much narrower, higher-confidence
band than similarity-based resolution (a face embedding, a fuzzy name
match), where the same numeric score means something less certain.
These are illustrative starting points, not tuned values - see the
architecture doc's "Threshold tuning" note: policy parameters, not fixed
constants.
"""
from app.ontology.schemas import EntityTypeDefinition

TARGET_PERSON = EntityTypeDefinition(
    name="TargetPerson",
    description=(
        "An individual identity, resolved primarily via face-based entity "
        "resolution (InsightFace/ArcFace embeddings). The platform's "
        "primary, most-developed Target Entity type."
    ),
    property_hints=["estimated_age", "estimated_gender", "embedding_uuid"],
    # Similarity-based resolution: a moderate score is still ambiguous,
    # so the gap between "confident" and "worth a human look" is wide.
    default_tau_high=0.75,
    default_tau_low=0.50,
)

VEHICLE = EntityTypeDefinition(
    name="Vehicle",
    description=(
        "A vehicle observed or associated with a Target, typically "
        "resolved via automatic number-plate recognition (ANPR/LPR)."
    ),
    property_hints=["plate_number", "make", "model", "color"],
    # Plate OCR is close to deterministic once characters are correctly
    # read - narrower band than face similarity, but not zero, since OCR
    # misreads happen.
    default_tau_high=0.90,
    default_tau_low=0.70,
)

PLACE = EntityTypeDefinition(
    name="Place",
    description=(
        "A location relevant to a Target - an address, venue, or "
        "geocoded point - typically resolved by lookup/geocoding rather "
        "than similarity matching."
    ),
    property_hints=["address", "coordinates", "place_type"],
    # Near-deterministic: a geocode either matches a known place or it's
    # a new one, so the queue band exists mainly for near-duplicate
    # addresses (typos, unit-number variants).
    default_tau_high=0.95,
    default_tau_low=0.85,
)

PHONE_NUMBER = EntityTypeDefinition(
    name="PhoneNumber",
    description=(
        "A phone number associated with a Target, typically arriving via "
        "call detail records (CDRs) - numbers, timestamps, cell-tower "
        "location, duration; not audio content."
    ),
    property_hints=["country_code", "carrier", "number_type"],
    # Exact-match identifier: the queue band exists almost entirely for
    # transcription/formatting variants, not genuine ambiguity.
    default_tau_high=0.95,
    default_tau_low=0.80,
)

ORGANIZATION = EntityTypeDefinition(
    name="Organization",
    description=(
        "A company, shell entity, or other organization associated with "
        "a Target, resolved via fuzzy name matching or exact registration "
        "numbers where available."
    ),
    property_hints=["legal_name", "registration_number", "jurisdiction"],
    # Fuzzy name matching carries real false-positive risk (similarly
    # named but unrelated entities) - closer to the Person profile than
    # to an exact-match identifier.
    default_tau_high=0.80,
    default_tau_low=0.55,
)

FINANCIAL_ACCOUNT = EntityTypeDefinition(
    name="FinancialAccount",
    description=(
        "A bank account, crypto wallet, or other financial instrument "
        "associated with a Target, typically resolved via exact account "
        "identifiers."
    ),
    property_hints=["account_number", "institution", "account_type"],
    default_tau_high=0.95,
    default_tau_low=0.80,
)

BUILTIN_ENTITY_TYPES: list[EntityTypeDefinition] = [
    TARGET_PERSON,
    VEHICLE,
    PLACE,
    PHONE_NUMBER,
    ORGANIZATION,
    FINANCIAL_ACCOUNT,
]
