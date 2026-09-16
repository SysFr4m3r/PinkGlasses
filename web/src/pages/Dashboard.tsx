import { useQuery } from "@tanstack/react-query";
import { api, Target } from "../api";
import { useNavigate } from "react-router-dom";
import { Stat, useToast } from "../components/ui";

export default function Dashboard({ scopeID }: { scopeID: string }) {
  const { data: sum } = useQuery({ queryKey: ["summary", scopeID], queryFn: () => api.summary(scopeID) });
  const { data: targets, refetch } = useQuery({ queryKey: ["targets", scopeID], queryFn: () => api.targets(scopeID) });
  const navigate = useNavigate();
  // Targets are added where they are scanned from: the Start-a-scan dialog.
  const addTargets = () => navigate("/runs?new=1");
  const toast = useToast();
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
                  <td>
                    {t.mode === "active"
                      ? <span className="badge b-active">active</span>
                      : <span className="badge">{t.mode.replace("_", " ")}</span>}
                  </td>
                  <td>{(t.tags ?? []).map((x) => <span key={x} className="pill">{x}</span>)}</td>
                  <td style={{ textAlign: "right" }}>
                    <button className="ghost sm" title="Take this target out of the company; future scans stop covering it"
                      onClick={() => remove(t)}>Remove</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

    </div>
  );
}
