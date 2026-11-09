"""Per-language doc-extraction backends.

Each module exposes `detect(source_dir) -> bool` and
`extract(source_dir, out_dir) -> ExtractResult`. That small, uniform
contract is what lets the orchestrator - and later a NATS job handler -
drive any of them identically, without caring how each language's own doc
tool actually works.
"""

from typing import Protocol

from ..models import ExtractResult
from . import golang, python, shell, typescript


class Extractor(Protocol):
    """Structural shape every per-language extractor module must satisfy.

    mypy matches a plain module against this Protocol by its top-level
    functions, so golang/typescript/python never need to subclass anything
    - they just need to define these two functions with these signatures.
    """

    def detect(self, source_dir: str) -> bool:
        """Reports whether `source_dir` has anything this extractor can document."""
        ...

    def extract(self, source_dir: str, out_dir: str) -> ExtractResult:
        """Runs this language's doc tool and writes output under `out_dir`."""
        ...


#: Language name -> the module that documents it. Add a new language by
#: adding one module here with a matching detect()/extract() pair.
REGISTRY: dict[str, Extractor] = {
    "go": golang,
    "typescript": typescript,
    "python": python,
    "shell": shell,
}
