import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, Worker } from "../api";
import { Modal, Stat, Badge, useToast, Spinner, InfoDot } from "../components/ui";

// Fleet management. Two kinds of worker, told apart by how they enrol:
//   local — containers beside the control plane. The standing one runs the
//           passive stages; a run's own fleet appears here while it runs and
//           is destroyed with the run. Nobody creates these by hand.
//   vps   — a rented box you enrol, an exit for active scans with its own
//           address and a true outside-in view.
export default function Fleet() {
  const toast = useToast();
  const { data: workers, refetch, isLoading } = useQuery({
    queryKey: ["workers"], queryFn: () => api.workers(), refetchInterval: 5000,
  });
  const [vpsOpen, setVpsOpen] = useState(false);

  const local = (workers ?? []).filter((w) => w.kind === "local");
  const remote = (workers ?? []).filter((w) => w.kind !== "local");

  async function act(w: Worker, action: string) {
    try {
      await api.workerAction(w.id, action);
      toast("ok", `${w.name}: ${action}`);
      refetch();
    } catch (e) {
      toast("err", String(e));
    }
  }

  const [pendingDelete, setPendingDelete] = useState<Worker | null>(null);

  async function confirmDelete() {
    const w = pendingDelete;
    if (!w) return;
    setPendingDelete(null);
    try {
      const r = await api.deleteWorker(w.id);
      toast(r.warning ? "err" : "ok", r.warning ?? `Removed ${w.name}`);
      refetch();
    } catch (e) {
      toast("err", String(e));
    }
  }

  return (
    <div>
      <div className="page-head">
        <div>
          <h2>Workers</h2>
          <div className="sub">
            Local workers are containers beside the control plane: the standing one runs
            passive discovery, and a run's own workers appear here while it runs. Remote
            workers are boxes you enrol, and the exits an active scan can leave from.
          </div>
        </div>
        {isLoading && <Spinner />}
      </div>

      <div className="cards">
        <Stat n={local.length} label="Local workers" hint="Standing worker for passive stages, plus any run's own fleet while it runs" />
        <Stat n={remote.length} label="External (VPS)" hint="Enrolled boxes · an exit for active scans" />
        <Stat n={(workers ?? []).filter((w) => w.status === "active").length} label="Active" hint="Currently leasing work" />
        <Stat n={(workers ?? []).reduce((a, w) => a + w.running_tasks, 0)} label="Running tasks" />
      </div>

      <div className="row">
        <button onClick={() => setVpsOpen(true)}>+ Add VPS worker</button>
      </div>

      <VPSModal open={vpsOpen} onClose={() => setVpsOpen(false)} />

      <WorkerTable title="Local workers" workers={local} act={act} onDelete={setPendingDelete} />
      <WorkerTable title="External (VPS) workers" workers={remote} act={act} onDelete={setPendingDelete} />

      <Modal
        title="Remove worker" open={!!pendingDelete} onClose={() => setPendingDelete(null)}
        footer={<>
          <button className="ghost" onClick={() => setPendingDelete(null)}>Cancel</button>
          <button className="danger" onClick={confirmDelete}>Remove worker</button>
        </>}
      >
        <p style={{ marginTop: 0 }}>
          Remove <strong>{pendingDelete?.name}</strong> from the fleet?
        </p>
        <p className="muted" style={{ fontSize: 13 }}>
          {pendingDelete?.kind === "local"
            ? "Its container is destroyed and its credential revoked. Any task it is running now is re-queued and picked up by another worker."
            : "Its credential is revoked, so the agent on that VPS will fail to reconnect and exit. Remember to also stop the container on the box itself."}
        </p>
      </Modal>
    </div>
  );
}

/* ---------- enroll a VPS ---------- */

function VPSModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const toast = useToast();
  const [name, setName] = useState("vps-1");
  const [cmd, setCmd] = useState("");
  const [busy, setBusy] = useState(false);

  async function mint() {
    setBusy(true);
    try {
      const r = await api.enrollToken({ kind: "vps", name, ttl_mins: 60 });
      setCmd(r.install_command);
    } catch (e) {
      toast("err", String(e));
    } finally {
      setBusy(false);
    }
  }

  function close() { setCmd(""); onClose(); }

  return (
    <Modal
      title="Add an external (VPS) worker" open={open} onClose={close}
      footer={cmd
        ? <button className="ghost" onClick={close}>Done</button>
        : (<>
            <button className="ghost" onClick={close}>Cancel</button>
            <button onClick={mint} disabled={busy || !name}>{busy ? "Minting…" : "Generate install command"}</button>
          </>)}
    >
      {!cmd ? (
        <div className="field">
          <label>Worker name</label>
          <input value={name} onChange={(e) => setName(e.target.value)} placeholder="vps-hetzner-fsn1" />
          <div className="hint">
            A VPS worker adds egress diversity and a true outside-in view: your own
            firewall and egress filtering quietly shape what a local worker sees. It
            connects outbound only (no inbound ports, works behind NAT); approve it here
            once it appears.
          </div>
        </div>
      ) : (
        <>
          <p style={{ marginTop: 0 }}>Run this on the VPS (needs Docker). The token is single-use and expires in 1 hour:</p>
          <div className="pre">{cmd}</div>
          <button className="ghost sm" style={{ marginTop: 10 }}
            onClick={() => { navigator.clipboard?.writeText(cmd); toast("ok", "Copied"); }}>
            Copy command
          </button>
        </>
      )}
    </Modal>
  );
}

/* ---------- table ---------- */

/* ---------- inline help ---------- */

function StatusHelp() {
  return (
    <InfoDot title="Worker statuses">
      <dl>
        <dt>pending</dt>
        <dd>Enrolled but not yet approved. Leases no work until you approve it.</dd>
        <dt>active</dt>
        <dd>Healthy and taking scan tasks.</dd>
        <dt>draining</dt>
        <dd>Finishing its current tasks, taking no new ones. A planned wind-down.</dd>
        <dt>quarantined</dt>
        <dd>Cut off after suspicious behaviour. Kept on the list so you can investigate.</dd>
        <dt>stale</dt>
        <dd>Stopped sending heartbeats. It returns to active by itself if it reconnects.</dd>
        <dt>revoked</dt>
        <dd>Credential invalidated; the agent fails its next connect and exits.</dd>
      </dl>
    </InfoDot>
  );
}

function ActionsHelp() {
  return (
    <InfoDot title="What these actions do">
      <dl>
        <dt>approve</dt>
        <dd>Lets a newly enrolled worker start taking scan tasks.</dd>
        <dt>drain — planned, graceful</dt>
        <dd>
          Stops new work but lets running tasks finish, so you can decommission, reboot or
          patch a worker without breaking a scan in progress. Reverse it with resume.
        </dd>
        <dt>quarantine — unplanned, defensive</dt>
        <dd>
          Cuts the worker off immediately when it may be compromised: no new work and no
          control channel. Its record and history are kept so you can see what it reported.
          Applied automatically if a worker reports assets outside its assigned target.
        </dd>
        <dt>resume</dt>
        <dd>Returns a draining or stale worker to active.</dd>
        <dt>remove</dt>
        <dd>
          Deletes the worker. A local worker's container is destroyed first, otherwise it
          would re-enroll and reappear.
        </dd>
      </dl>
    </InfoDot>
  );
}

function WorkerTable({
  title, workers, act, onDelete,
}: {
  title: string; workers: Worker[];
  act: (w: Worker, a: string) => void;
  onDelete: (w: Worker) => void;
}) {
  return (
    <>
      <div className="section-title">{title}</div>
      {workers.length === 0 ? (
        <div className="empty">None enrolled.</div>
      ) : (
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Status<StatusHelp /></th>
                <th>Egress IP</th><th>Capabilities</th>
                <th>Tools</th><th>Load</th>
                <th>Actions<ActionsHelp /></th>
              </tr>
            </thead>
            <tbody>
              {workers.map((w) => (
                <tr key={w.id}>
                  <td>
                    {w.name}
                    {w.run_scoped && (
                      <span
                        className="pill"
                        style={{ marginLeft: 6, fontSize: 10.5 }}
                        title="This worker belongs to one scan. It takes that scan's tasks only, and is destroyed when the scan ends."
                      >one scan</span>
                    )}
                    <div className="muted" style={{ fontSize: 11.5 }}>{w.kind} · {w.agent_version || "—"}</div>
                  </td>
                  <td><Badge status={w.status} /></td>
                  <td className="mono">{w.egress_ip ?? "—"}</td>
                  <td>{(w.capabilities ?? []).map((c) => <span key={c} className="pill">{c}</span>)}</td>
                  <td className="muted" style={{ fontSize: 11.5 }}>
                    {Object.keys(w.tools ?? {}).slice(0, 4).join(", ") || "—"}
                  </td>
                  <td className="muted">{w.running_tasks}/{w.max_concurrency}</td>
                  <td>
                    {w.status === "pending" && <button className="sm" onClick={() => act(w, "approve")}>approve</button>}
                    {w.status === "active" && <button className="ghost sm" onClick={() => act(w, "drain")}>drain</button>}
                    {(w.status === "draining" || w.status === "stale") &&
                      <button className="sm" onClick={() => act(w, "resume")}>resume</button>}
                    {w.status !== "quarantined" && w.status !== "revoked" &&
                      <button className="ghost sm" style={{ marginLeft: 6 }} onClick={() => act(w, "quarantine")}>quarantine</button>}
                    <button className="danger sm" style={{ marginLeft: 6 }}
                      title="Remove this worker from the fleet"
                      onClick={() => onDelete(w)}>remove</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}
