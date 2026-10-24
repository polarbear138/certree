import React, { useEffect, useRef } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { NavLink } from "react-router-dom";
import { X } from "lucide-react";
import { api, Row, TreeNode } from "./api";

export function targetIDs(node: TreeNode): string[] {
  return node.type === "target"
    ? [node.entity_id]
    : node.children.flatMap(targetIDs);
}

export function SelectionCheckbox({
  ids,
  selected,
  label,
  onToggle,
}: {
  ids: string[];
  selected: Set<string>;
  label: string;
  onToggle: (ids: string[]) => void;
}) {
  const ref = useRef<HTMLInputElement>(null);
  const count = ids.filter((id) => selected.has(id)).length;
  useEffect(() => {
    if (ref.current)
      ref.current.indeterminate = count > 0 && count < ids.length;
  }, [count, ids.length]);
  return (
    <input
      ref={ref}
      type="checkbox"
      className="tree-checkbox"
      tabIndex={-1}
      aria-label={label}
      checked={ids.length > 0 && count === ids.length}
      disabled={!ids.length}
      onClick={(e) => e.stopPropagation()}
      onKeyDown={(e) => e.stopPropagation()}
      onChange={() => onToggle(ids)}
    />
  );
}

export function ExpiryBar({
  value,
  compact = false,
  summary = false,
}: {
  value?: Row;
  compact?: boolean;
  summary?: boolean;
}) {
  const known = !!value?.not_after;
  const state = value?.state || "Unknown";
  const label = !known
    ? state
    : state === "Expired"
      ? "Expired"
      : `${value.days_remaining}d left`;
  const percent = known
    ? Math.max(0, Math.min(100, Number(value.remaining_percent) || 0))
    : 0;
  const title = known
    ? `Last verified certificate · expires ${new Date(value.not_after).toLocaleString()}${summary ? ` · ${value.known}/${value.total} targets have a known applied certificate` : ""}`
    : state === "Not deployed"
      ? "No certificate has been deployed and verified"
      : "Applied certificate expiry is unavailable";
  return (
    <div
      className={`expiry-meter ${compact ? "compact" : ""} expiry-${state.toLowerCase().replaceAll(" ", "-")}`}
      title={title}
    >
      <span>
        {summary && known ? "Earliest: " : ""}
        {label}
      </span>
      {known && (
        <progress
          max={100}
          value={percent}
          aria-label={title}
          aria-valuetext={label}
        />
      )}
      {!compact && known && (
        <small>Expires {new Date(value.not_after).toLocaleDateString()}</small>
      )}
      {summary && !compact && (
        <small>
          {value?.total || 0} targets · {value?.known || 0} applied ·{" "}
          {value?.not_deployed || 0} not deployed · {value?.unknown || 0}{" "}
          unknown
        </small>
      )}
    </div>
  );
}

export function BulkDeployDialog({
  ids,
  onClose,
  onQueued,
}: {
  ids: string[];
  onClose: () => void;
  onQueued: (ids: string[]) => void;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const client = useQueryClient();
  const preview = useQuery({
    queryKey: ["bulk-preview", ids],
    queryFn: () =>
      api<{ items: Row[] }>("deployments/bulk/preview", { target_ids: ids }),
    refetchOnWindowFocus: false,
    staleTime: Infinity,
  });
  const submit = useMutation({
    mutationFn: (items: Row[]) =>
      api<{ items: Row[] }>("deployments/bulk", {
        items: items.map(({ target_id, version_id }) => ({
          target_id,
          version_id,
        })),
      }),
    onSuccess: (result) => {
      onQueued(
        result.items
          .filter((item) => item.job_id)
          .map((item) => item.target_id),
      );
      client.invalidateQueries({
        predicate: (query) => query.queryKey[0] !== "bulk-preview",
      });
    },
  });
  useEffect(() => {
    ref.current?.showModal();
  }, []);
  const ready = (preview.data?.items || []).filter((item) => item.ready);
  const results = new Map(
    (submit.data?.items || []).map((item) => [item.target_id, item]),
  );
  const items = (preview.data?.items || []).map(
    (item) => results.get(item.target_id) || item,
  );
  const blockedAtReview = (preview.data?.items || []).filter(
    (item) => !item.ready,
  ).length;
  const error = preview.error || submit.error;
  return (
    <dialog
      ref={ref}
      className="modal bulk-modal"
      aria-labelledby="bulk-title"
      onCancel={(e) => {
        if (submit.isPending) e.preventDefault();
        else onClose();
      }}
    >
      <div className="modal-head">
        <h2 id="bulk-title">
          {submit.isSuccess
            ? "Bulk deployment results"
            : "Review bulk deployment"}
        </h2>
        <button
          aria-label="Close"
          disabled={submit.isPending}
          onClick={onClose}
        >
          <X size={18} />
        </button>
      </div>
      <div className="bulk-content">
        <p>
          Deploy the approved versions shown below. Targets on the same server
          run one at a time. No certificate approval is granted by this action.
        </p>
        {error && (
          <p role="alert" className="notice error">
            {error.message}
          </p>
        )}
        {preview.isPending ? (
          <p role="status">Checking selected targets…</p>
        ) : (
          <div className="bulk-table">
            <table>
              <thead>
                <tr>
                  <th>Server / endpoint</th>
                  <th>Certificate / reviewed version</th>
                  <th>Result</th>
                </tr>
              </thead>
              <tbody>
                {items.map((item) => (
                  <tr key={item.target_id}>
                    <td>
                      <strong>{item.server_name || item.target_id}</strong>
                      <small>{item.sni}</small>
                    </td>
                    <td>
                      {item.certificate_name || "—"}
                      <small className="mono">
                        {item.version_id || "No approved version"}
                      </small>
                      {item.fingerprint && (
                        <small className="mono wrap">
                          SHA-256 {item.fingerprint}
                        </small>
                      )}
                      {item.not_after && (
                        <small>
                          Expires {new Date(item.not_after).toLocaleString()}
                        </small>
                      )}
                    </td>
                    <td>
                      {item.job_id ? (
                        <>
                          <strong>Queued</strong>
                          <small className="mono">
                            {item.job_id.slice(0, 12)}
                          </small>
                        </>
                      ) : item.error ? (
                        <span className="error-text">{item.error}</span>
                      ) : (
                        "Ready"
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {submit.isSuccess && (
          <p role="status">
            {submit.data.items.filter((item) => item.job_id).length} queued ·{" "}
            {blockedAtReview} blocked at review ·{" "}
            {submit.data.items.filter((item) => !item.job_id).length} rejected
            at submission. Unqueued targets remain selected.
          </p>
        )}
      </div>
      <div className="bulk-actions">
        <button onClick={onClose} disabled={submit.isPending}>
          {submit.isSuccess ? "Close" : "Cancel"}
        </button>
        {submit.isSuccess ? (
          <NavLink
            className="button primary"
            to="/history?tab=deployments"
            onClick={onClose}
          >
            View deployment history
          </NavLink>
        ) : (
          <button
            className="primary"
            disabled={
              !ready.length ||
              preview.isPending ||
              submit.isPending ||
              !!preview.error
            }
            onClick={() => submit.mutate(ready)}
          >
            {submit.isPending
              ? "Queueing…"
              : `Deploy ${ready.length} ready targets`}
          </button>
        )}
      </div>
    </dialog>
  );
}
