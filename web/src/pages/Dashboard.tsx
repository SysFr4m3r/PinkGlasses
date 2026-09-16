import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, Target } from "../api";
import { Modal, Stat, useToast } from "../components/ui";

export default function Dashboard({ scopeID }: { scopeID: string }) {
  const { data: sum } = useQuery({ queryKey: ["summary", scopeID], queryFn: () => api.summary(scopeID) });
  const { data: targets, refetch } = useQuery({ queryKey: ["targets", scopeID], queryFn: () => api.targets(scopeID) });
  const [open, setOpen] = useState(false);
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
        <button onClick={() => setOpen(true)}>+ Add targets</button>
      </div>

      {(targets ?? []).length === 0 ? (
        <div className="empty">
          <p>No targets yet. Add a domain or CIDR to start discovering.</p>
          <button onClick={() => setOpen(true)}>Add your first target</button>
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
                  <td title={t.mode === "active" && t.authorized_by
                    ? `Active scanning authorized by ${t.authorized_by}${t.authorized_at ? " on " + new Date(t.authorized_at).toLocaleDateString() : ""}`
                    : "Passive discovery only; nothing is sent to it"}>
                    {t.mode === "active" && t.authorized_by
                      ? <span className="badge b-active">active</span>
                      : <span className="badge">{t.mode === "exclude" ? "excluded" : "passive only"}</span>}
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

      <AddTargets scopeID={scopeID} open={open} onClose={() => setOpen(false)} onDone={refetch} />
      {editing && (
        <EditTarget scopeID={scopeID} target={editing} onClose={() => setEditing(null)} onDone={refetch} />
      )}
    </div>
  );
}

/**
 * Edit a target — the same three things as when it was added: the host or
 * range, its tags, and whether active scanning is authorized. Nothing about
 * how a scan runs belongs here; that is the Start-a-scan dialog.
 */
function EditTarget({ scopeID, target, onClose, onDone }: {
  scopeID: string; target: Target; onClose: () => void; onDone: () => void;
}) {
  const toast = useToast();
  const [value, setValue] = useState(target.value);
  const [active, setActive] = useState(target.mode === "active" && !!target.authorized_by);
  const [tags, setTags] = useState((target.tags ?? []).join(", "));
  const [busy, setBusy] = useState(false);

  async function save() {
    setBusy(true);
    try {
      await api.patchTarget(scopeID, target.id, {
        ...(value.trim() !== target.value ? { value: value.trim() } : {}),
        mode: active ? "active" : "passive_only",
        authorize: active,
        tags: tags.split(/[\s,]+/).filter(Boolean),
      });
      toast("ok", `${value.trim()} updated`);
      onDone(); onClose();
    } catch (e) {
      toast("err", String(e).replace(/^Error:\s*/, ""));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal
      title={`Edit ${target.value}`} open onClose={onClose}
      footer={<>
        <button className="ghost" onClick={onClose}>Cancel</button>
        <button onClick={save} disabled={busy || !value.trim()}>{busy ? "Saving…" : "Save"}</button>
      </>}
    >
      <div className="field">
        <label>Domain, IP or CIDR</label>
        <input className="mono" value={value} onChange={(e) => setValue(e.target.value)} style={{ width: "100%" }} />
        <div className="hint">The kind is detected automatically. What earlier scans found stays in the inventory.</div>
      </div>
      <div className="field">
        <label>Tags (optional)</label>
        <input value={tags} onChange={(e) => setTags(e.target.value)} placeholder="production, eu" />
        <div className="hint">Tags group targets; a run can be started over just one tag through the API.</div>
      </div>
      <label className="check" style={{ cursor: "pointer" }}>
        <input type="checkbox" checked={active} onChange={(e) => setActive(e.target.checked)} />
        <span>
          <strong>Authorize active scanning</strong>
          <div className="hint" style={{ marginTop: 3 }}>
            {target.authorized_by
              ? `Currently authorized by ${target.authorized_by}${target.authorized_at ? " on " + new Date(target.authorized_at).toLocaleDateString() : ""}. Untick for passive-only discovery.`
              : "Leave unticked for passive-only discovery (CT logs, DNS, public APIs — no packets sent to the target). Only tick this for infrastructure you are authorized to scan."}
          </div>
        </span>
      </label>
    </Modal>
  );
}

/** Add targets to the company. Just the list — nothing here is about a scan. */
function AddTargets({
  scopeID, open, onClose, onDone,
}: { scopeID: string; open: boolean; onClose: () => void; onDone: () => void }) {
  const toast = useToast();
  const [text, setText] = useState("");
  const [active, setActive] = useState(false);
  const [tags, setTags] = useState("");
  const [busy, setBusy] = useState(false);
  const values = text.split(/[\s,]+/).map((v) => v.trim()).filter(Boolean);

  async function save() {
    setBusy(true);
    try {
      await api.addTarget(scopeID, {
        values,
        mode: active ? "active" : "passive_only",
        authorize: active,
        tags: tags.split(/[\s,]+/).filter(Boolean),
      });
      toast("ok", `Added ${values.length} target${values.length === 1 ? "" : "s"}`);
      setText(""); setTags(""); setActive(false);
      onDone(); onClose();
    } catch (e) {
      toast("err", String(e).replace(/^Error:\s*/, ""));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal
      title="Add targets" open={open} onClose={onClose}
      footer={<>
        <button className="ghost" onClick={onClose}>Cancel</button>
        <button onClick={save} disabled={busy || !values.length}>
          {busy ? "Saving…" : `Add ${values.length || ""} target${values.length === 1 ? "" : "s"}`}
        </button>
      </>}
    >
      <div className="field">
        <label>Domains, IPs or CIDRs</label>
        <textarea rows={5} style={{ width: "100%" }} value={text} onChange={(e) => setText(e.target.value)}
          placeholder={"example.com\nshop.example.com\n203.0.113.0/24"} />
        <div className="hint">One per line, or comma-separated. The kind is detected automatically.</div>
      </div>
      <div className="field">
        <label>Tags (optional)</label>
        <input value={tags} onChange={(e) => setTags(e.target.value)} placeholder="production, eu" />
        <div className="hint">Tags group targets; a run can be started over just one tag through the API.</div>
      </div>
      <label className="check" style={{ cursor: "pointer" }}>
        <input type="checkbox" checked={active} onChange={(e) => setActive(e.target.checked)} />
        <span>
          <strong>Authorize active scanning</strong>
          <div className="hint" style={{ marginTop: 3 }}>
            Leave unticked for passive-only discovery (CT logs, DNS, public APIs — no packets sent to
            the target). Only tick this for infrastructure you are authorized to scan.
          </div>
        </span>
      </label>
    </Modal>
  );
}
