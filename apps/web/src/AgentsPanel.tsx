import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, Row } from "./api";

export function AgentsPanel() {
  const qc = useQueryClient();
  const [environment, setEnvironment] = useState("");
  const [registration, setRegistration] = useState<{
    token: string;
    expires: number;
  } | null>(null);
  const environments = useQuery({
    queryKey: ["environments"],
    queryFn: () => api<Row[]>("environments"),
  });
  const agents = useQuery({
    queryKey: ["agents"],
    queryFn: () => api<Row[]>("agents"),
    refetchInterval: 5000,
  });
  const enroll = useMutation({
    mutationFn: () =>
      api<{ token: string; expires: number }>("agents/enrollment", {
        environment_id: environment,
      }),
    onSuccess: (value) => {
      setRegistration(value);
      qc.invalidateQueries({ queryKey: ["audit-logs"] });
    },
  });
  const revoke = useMutation({
    mutationFn: (id: string) => api(`agents/${id}/revoke`, {}),
    onSuccess: () => qc.invalidateQueries(),
  });
  const error =
    agents.error || environments.error || enroll.error || revoke.error;
  return (
    <section className="panel settings-panel">
      <h2>Client agents</h2>
      <p>
        Enroll a Linux server, then configure its Nginx certificate paths with{" "}
        <code>certree settings</code>. Agents connect to this server over HTTPS.
      </p>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          setRegistration(null);
          enroll.mutate();
        }}
      >
        <label>
          Allowed environment
          <select
            required
            value={environment}
            onChange={(event) => {
              setEnvironment(event.target.value);
              setRegistration(null);
            }}
          >
            <option value="">Select an environment</option>
            {environments.data?.map((item) => (
              <option key={item.id} value={item.id}>
                {item.name} · {item.organization_id.slice(0, 8)}
              </option>
            ))}
          </select>
        </label>
        <button className="primary" disabled={!environment || enroll.isPending}>
          Create registration key
        </button>
      </form>
      {error && (
        <div role="alert" className="notice error">
          {error.message}
        </div>
      )}
      {registration && (
        <div className="notice" role="status">
          <p>
            Single-use key · expires{" "}
            {new Date(registration.expires * 1000).toLocaleTimeString()}. Enter
            it at the agent's hidden registration prompt.
          </p>
          <code className="wrap">{registration.token}</code>
          <p>
            <button onClick={() => setRegistration(null)}>Hide key</button>
          </p>
        </div>
      )}
      {!agents.data?.length ? (
        <p>
          No agents registered yet. Create an organization and environment in
          Tree Explorer first.
        </p>
      ) : (
        <table>
          <thead>
            <tr>
              <th>Agent</th>
              <th>Connection</th>
              <th>Last seen</th>
              <th>Access</th>
            </tr>
          </thead>
          <tbody>
            {agents.data.map((item) => (
              <tr key={item.id}>
                <td>
                  {item.name}
                  <small className="mono">{item.id.slice(0, 12)}</small>
                </td>
                <td>
                  {item.revoked
                    ? "Revoked"
                    : Date.now() / 1000 - item.last_seen < 90
                      ? "Connected"
                      : "Offline"}
                </td>
                <td>
                  {item.last_seen
                    ? new Date(item.last_seen * 1000).toLocaleString()
                    : "Never"}
                </td>
                <td>
                  <button
                    disabled={!!item.revoked || revoke.isPending}
                    onClick={() => {
                      if (
                        window.confirm(
                          `Revoke ${item.name}? Further connections will be rejected. An already downloaded job may still finish locally.`,
                        )
                      )
                        revoke.mutate(item.id);
                    }}
                  >
                    Revoke access
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <p>
        Agent targets appear in Tree Explorer and Servers. Approve a certificate
        version there before deployment. A reload is successful only after
        central live TLS verification.
      </p>
    </section>
  );
}
