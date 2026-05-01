from __future__ import annotations
import os
import subprocess
import threading
import time
import uuid
from enum import Enum
from .manifest import AgentManifest
from .events import (
    EventStreamer,
    parse_agent_output_line,
    parse_tool_call_line,
    parse_tool_result_line,
    parse_user_input_line,
)


class AgentState(Enum):
    PENDING = "pending"
    RUNNING = "running"
    STOPPED = "stopped"
    CRASHED = "crashed"


class AgentProcess:
    def __init__(self, manifest: AgentManifest, streamer: EventStreamer, daemon=None, scenario_id: str | None = None):
        self.manifest = manifest
        self.streamer = streamer
        self._daemon = daemon
        self._scenario_id = scenario_id
        self.state = AgentState.PENDING
        self._proc: subprocess.Popen | None = None
        self._agent_id: str | None = None
        self._session_id: str | None = None
        self._restart_count = 0
        self.started_at: float | None = None

    @property
    def pid(self) -> int | None:
        return self._proc.pid if self._proc else None

    @property
    def agent_id(self) -> str | None:
        return self._agent_id

    @property
    def session_id(self) -> str | None:
        return self._session_id

    @property
    def scenario_id(self) -> str | None:
        return self._scenario_id

    @property
    def name(self) -> str:
        return self.manifest.name

    def start(self) -> None:
        self.state = AgentState.RUNNING
        self.started_at = time.time()
        self._session_id = self._build_session_id()
        if self._daemon and self._daemon._available:
            self._agent_id = self._daemon.run_agent(self.manifest)
            if self._agent_id:
                self.streamer.emit(
                    self.name,
                    "session_start",
                    {
                        "launch_mode": "daemon",
                        "command": self.manifest.command,
                        "allowed_hosts": self.manifest.allowed_hosts,
                        "mode": self.manifest.mode,
                    },
                    session_id=self._session_id,
                    scenario_id=self._scenario_id,
                    agent_id=self._agent_id,
                )
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
        self.streamer.emit(
            self.name,
            "session_start",
            {
                "launch_mode": "local",
                "command": self.manifest.command,
                "allowed_hosts": self.manifest.allowed_hosts,
                "mode": self.manifest.mode,
                "pid": self.pid,
            },
            session_id=self._session_id,
            scenario_id=self._scenario_id,
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

    def _build_session_id(self) -> str:
        return f"{self.name}-{uuid.uuid4().hex[:12]}"

    def _read_output(self):
        for line in self._proc.stdout:
            line = line.rstrip()
            print(f"[{self.name}] {line}", flush=True)
            self.streamer.emit(
                self.name,
                "stdout",
                {"line": line},
                session_id=self._session_id,
                scenario_id=self._scenario_id,
                agent_id=self._agent_id,
            )
            user_input = parse_user_input_line(line)
            if user_input:
                self.streamer.emit(
                    self.name,
                    "user_input",
                    user_input,
                    session_id=self._session_id,
                    scenario_id=self._scenario_id,
                    agent_id=self._agent_id,
                )
            elif line.startswith("[TOOL]"):
                self.streamer.emit(
                    self.name,
                    "tool_call",
                    parse_tool_call_line(line),
                    session_id=self._session_id,
                    scenario_id=self._scenario_id,
                    agent_id=self._agent_id,
                )
            elif line.startswith("[RESULT]"):
                self.streamer.emit(
                    self.name,
                    "tool_result",
                    parse_tool_result_line(line),
                    session_id=self._session_id,
                    scenario_id=self._scenario_id,
                    agent_id=self._agent_id,
                )
            else:
                agent_output = parse_agent_output_line(line)
                if agent_output:
                    self.streamer.emit(
                        self.name,
                        "agent_output",
                        agent_output,
                        session_id=self._session_id,
                        scenario_id=self._scenario_id,
                        agent_id=self._agent_id,
                    )
        rc = self._proc.wait()
        if rc != 0:
            self.state = AgentState.CRASHED
            self.streamer.emit(
                self.name,
                "crashed",
                {"exit_code": rc},
                session_id=self._session_id,
                scenario_id=self._scenario_id,
                agent_id=self._agent_id,
            )
        else:
            self.state = AgentState.STOPPED
            self.streamer.emit(
                self.name,
                "stopped",
                {"exit_code": rc},
                session_id=self._session_id,
                scenario_id=self._scenario_id,
                agent_id=self._agent_id,
            )
