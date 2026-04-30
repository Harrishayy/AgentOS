from __future__ import annotations
from dataclasses import dataclass, field
from pathlib import Path
import yaml


@dataclass
class AgentManifest:
    name: str
    command: list[str]
    allowed_hosts: list[str]           # required; [] = deny all egress
    allowed_paths: list[str]           # required; [] until P1 ships path enforcement
    env: dict[str, str] = field(default_factory=dict)
    mode: str = "enforce"              # "enforce" | "audit"
    allowed_bins: list[str] = field(default_factory=list)
    forbidden_caps: list[str] = field(default_factory=list)
    working_dir: str | None = None


_REQUIRED = ("name", "command", "allowed_hosts", "allowed_paths")


def load_manifest(path: str | Path) -> AgentManifest:
    with open(path) as f:
        data = yaml.safe_load(f)
    for key in _REQUIRED:
        if key not in data:
            raise ValueError(
                f"Manifest '{path}' is missing required field '{key}'. "
                f"Use an empty list ([]) to explicitly allow nothing."
            )
    cmd = data["command"]
    return AgentManifest(
        name=data["name"],
        command=cmd if isinstance(cmd, list) else cmd.split(),
        allowed_hosts=data["allowed_hosts"],
        allowed_paths=data["allowed_paths"],
        env=data.get("env", {}),
        mode=data.get("mode", "enforce"),
        allowed_bins=data.get("allowed_bins", []),
        forbidden_caps=data.get("forbidden_caps", []),
        working_dir=data.get("working_dir"),
    )
