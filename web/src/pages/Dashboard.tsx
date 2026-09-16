import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, Target } from "../api";
import { useNavigate } from "react-router-dom";
import { Modal, Stat, useToast } from "../components/ui";

export default function Dashboard({ scopeID }: { scopeID: string }) {
  const { data: sum } = useQuery({ queryKey: ["summary", scopeID], queryFn: () => api.summary(scopeID) });
  const { data: targets, refetch } = useQuery({ queryKey: ["targets", scopeID], queryFn: () => api.targets(scopeID) });
  const navigate = useNavigate();
  // Targets are added where they are scanned from: the Start-a-scan dialog.
  const addTargets = () => navigate("/runs?new=1");
  const toast = useToast();
  const [editing, setEditing] = useState<Target | null>(null);
  async function remove(t: Target) {
    try {
      await api.deleteTarget(scopeID, t.id);
      toast("ok", `${t.value} removed — what earlier scans found under it stays in the inventory`);
      refetch();
    } catch (e) {
      toast("err", String(e).replace(/^Error:\s*/, ""));
    }
  }

  return (
    <div>
      <div className="page-head">
        <div>
          <h2>Dashboard</h2>
          <div className="sub">Your external attack surface at a glance.</div>
        </div>
      </div>

      <div className="cards">
        <Stat n={sum?.domains_resolving} label="Resolving names"
              hint={sum && sum.domains > sum.domains_resolving
                ? `${(sum.domains - sum.domains_resolving).toLocaleString()} more never resolved`
                : undefined} />
        <Stat n={sum?.ips} label="Hosts" />
        <Stat n={sum?.services} label="Services" to="/search?q=product%3A*"
          title="Open Search with every service listed and summarized by product, port and technology" />
        <Stat n={sum?.open_findings} label="Open findings" />
      </div>

      <div className="page-head">
        <div className="section-title" style={{ margin: 0 }}>Scope targets</div>
        <button onClick={addTargets} title="Targets are added in the Start-a-scan dialog, where you also choose what to scan">+ Add targets</button>
      </div>

      {(targets ?? []).length === 0 ? (
        <div className="empty">
          <p>No targets yet. Add a domain or CIDR to start discovering.</p>
          <button onClick={addTargets}>Add your first target</button>
        </div>
      ) : (
        <div className="table-wrap">
          <table>
            <thead><tr><th>Value</th><th>Kind</th><th>Mode</th><th>Tags</th><th></th></tr></thead>
            <tbody>
              {(targets ?? []).map((t: Target) => (
                <tr key={t.id}>
                  <td className="mono">{t.value}</td>
                  <td className="muted">{t.kind}</td>
                  <td title={t.mode === "active"
                    ? (t.authorized_by ? `Active scanning authorized by ${t.authorized_by}${t.authorized_at ? " on " + new Date(t.authorized_at).toLocaleDateString() : ""}` : "Active, but no authorization recorded — active stages skip it")
                    : t.mode === "exclude" ? "Never scanned, and left out of every run" : "Passive discovery only; nothing is sent to it"}>
                    {t.mode === "active"
                      ? <span className={"badge" + (t.authorized_by ? " b-active" : "")}>{t.authorized_by ? "active" : "active · unauthorized"}</span>
                      : <span className="badge">{t.mode.replace("_", " ")}</span>}
                  </td>
                  <td>{(t.tags ?? []).map((x) => <span key={x} className="pill">{x}</span>)}</td>
                  <td style={{ textAlign: "right", whiteSpace: "nowrap" }}>
                    <button className="ghost sm" title="Change how this target is scanned and its tags"
                      onClick={() => setEditing(t)}>Edit</button>
                    <button className="ghost sm" title="Take this target out of the company; future scans stop covering it"
                      onClick={() => remove(t)}>Remove</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {editing && (
        <EditTarget scopeID={scopeID} target={editing} onClose={() => setEditing(null)} onDone={refetch} />
      )}
    </div>
  );
}

/**
 * Edit a target's mode, authorization and tags. The value is its identity, so
 * changing it is remove and add; everything else about how it is scanned is
 * here. Authorization is an explicit tick: switching to active without it
 * leaves the target active but unauthorized, which the active stages skip.
 */
function EditTarget({ scopeID, target, onClose, onDone }: {
  scopeID: string; target: Target; onClose: () => void; onDone: () => void;
}) {
  const toast = useToast();
  const [mode, setMode] = useState(target.mode);
  const [authorize, setAuthorize] = useState(!!target.authorized_by);
  const [tags, setTags] = useState((target.tags ?? []).join(", "));
  const [busy, setBusy] = useState(false);

  async function save() {
    setBusy(true);
    try {
      await api.patchTarget(scopeID, target.id, {
        mode,
        tags: tags.split(/[\s,]+/).filter(Boolean),
        ...(mode === "active" ? { authorize } : {}),
      });
      toast("ok", `${target.value} updated`);
      onDone(); onClose();
    } catch (e) {
      toast("err", String(e).replace(/^Error:\s*/, ""));
    } finally {
      setBusy(false);
    }
  }

  const MODES: { id: string; label: string; hint: string }[] = [
    { id: "passive_only", label: "Passive only", hint: "Discovery and resolution from public sources. Nothing is sent to it." },
    { id: "active", label: "Active", hint: "Port scan, probing, screenshots, directory search and the vulnerability check — only if authorized below." },
    { id: "exclude", label: "Exclude", hint: "Left out of every run, even passive ones. Keeps the record that it is yours." },
  ];

  return (
    <Modal
      title={`Edit ${target.value}`} open onClose={onClose}
      footer={<>
        <button className="ghost" onClick={onClose}>Cancel</button>
        <button onClick={save} disabled={busy}>{busy ? "Saving…" : "Save"}</button>
      </>}
    >
      <p className="muted" style={{ marginTop: 0, fontSize: 13 }}>
        <span className="mono">{target.value}</span> · {target.kind}. The value itself is the target's
        identity; to change it, remove this target and add the new one.
      </p>
      <div className="param-label" style={{ minWidth: 0, marginBottom: 6 }}>How it is scanned</div>
      {MODES.map((m) => (
        <label key={m.id} className="check" style={{ cursor: "pointer" }}>
          <input type="radio" name="tmode" checked={mode === m.id} onChange={() => setMode(m.id)} />
          <span><strong>{m.label}</strong><div className="hint" style={{ marginTop: 2 }}>{m.hint}</div></span>
        </label>
      ))}
      {mode === "active" && (
        <label className="check" style={{ cursor: "pointer", marginLeft: 22 }}>
          <input type="checkbox" checked={authorize} onChange={(e) => setAuthorize(e.target.checked)} />
          <span>
            <strong>Authorize active scanning</strong>
            <div className="hint" style={{ marginTop: 2 }}>
              {target.authorized_by
                ? `Currently authorized by ${target.authorized_by}${target.authorized_at ? " on " + new Date(target.authorized_at).toLocaleDateString() : ""}. Untick to revoke.`
                : "Recorded with your name and the time. Tick it only for infrastructure you are authorized to scan."}
            </div>
          </span>
        </label>
      )}
      <div className="field" style={{ marginTop: 12 }}>
        <label>Tags</label>
        <input value={tags} onChange={(e) => setTags(e.target.value)} placeholder="production, eu" style={{ width: "100%" }} />
        <div className="hint">Comma-separated. Tags group targets; a run can be started over just one tag through the API.</div>
      </div>
    </Modal>
  );
}
