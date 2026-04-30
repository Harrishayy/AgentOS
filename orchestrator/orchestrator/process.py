from __future__ import annotations
import os
import subprocess
import threading
import time
from enum import Enum
from .manifest import AgentManifest
from .events import EventStreamer, parse_tool_call_line


class AgentState(Enum):
    PENDING = "pending"
    RUNNING = "running"
    STOPPED = "stopped"
    CRASHED = "crashed"


class AgentProcess:
    def __init__(self, manifest: AgentManifest, streamer: EventStreamer, daemon=None):
        self.manifest = manifest
        self.streamer = streamer
        self._daemon = daemon
        self.state = AgentState.PENDING
        self._proc: subprocess.Popen | None = None
        self._agent_id: str | None = None
        self._restart_count = 0
        self.started_at: float | None = None

    @property
    def pid(self) -> int | None:
        return self._proc.pid if self._proc else None

    @property
    def agent_id(self) -> str | None:
        return self._agent_id

    @property
    def name(self) -> str:
        return self.manifest.name

    def start(self) -> None:
        self.state = AgentState.RUNNING
        self.started_at = time.time()
        if self._daemon and self._daemon._available:
            self._agent_id = self._daemon.run_agent(self.manifest)
            if self._agent_id:
                print(f"[orchestrator] '{self.name}' running in daemon sandbox (id={self._agent_id})", flush=True)
                # LLM-level stdout events not available in daemon mode until P2 ships
                # stdout relay via StreamEvents. Lifecycle events (agent.exited/crashed)
                # come from the daemon's event stream.
                return
        self._start_local()

    def _start_local(self):
        """Fallback: spawn directly when daemon is unavailable (stub / local dev)."""
        env = {**os.environ, **self.manifest.env}
        self._proc = subprocess.Popen(
            self.manifest.command,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
            bufsize=1,
            env=env,
        )
        threading.Thread(target=self._read_output, daemon=True).start()

    def stop(self):
        if self._agent_id and self._daemon:
            self._daemon.stop_agent(self._agent_id)
        elif self._proc and self._proc.poll() is None:
            self._proc.terminate()
            try:
                self._proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                self._proc.kill()
        self.state = AgentState.STOPPED

    def is_alive(self) -> bool:
        if self._agent_id:
            return self.state == AgentState.RUNNING
        return self._proc is not None and self._proc.poll() is None

    def wait(self, timeout: float | None = None) -> int | None:
        if self._proc:
            return self._proc.wait(timeout=timeout)
        return None

    def _read_output(self):
        for line in self._proc.stdout:
            line = line.rstrip()
            print(f"[{self.name}] {line}", flush=True)
            self.streamer.emit(self.name, "stdout", {"line": line})
            if line.startswith("[TOOL]"):
                self.streamer.emit(self.name, "tool_call", parse_tool_call_line(line))
        rc = self._proc.wait()
        if rc != 0:
            self.state = AgentState.CRASHED
            self.streamer.emit(self.name, "crashed", {"exit_code": rc})
        else:
            self.state = AgentState.STOPPED
            self.streamer.emit(self.name, "stopped", {"exit_code": rc})
