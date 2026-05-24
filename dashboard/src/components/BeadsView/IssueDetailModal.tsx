import { useState } from 'react'
import type { BeadsComment, BeadsIssue, BeadsIssueDetail } from './types'
import { addIssueComment, useIssueComments, useIssueDetail } from './hooks'

interface IssueDetailModalProps {
  projectPath: string
  issue: BeadsIssue
  onClose: () => void
  onIssueUpdated?: () => void
}

const COMMON_FIELDS = new Set([
  'id',
  'title',
  'status',
  'priority',
  'type',
  'issue_type',
  'assignee',
  'owner',
  'labels',
  'description',
  'design',
  'acceptance',
  'acceptance_criteria',
  'notes',
  'created',
  'created_at',
  'updated',
  'updated_at',
  'dependencies',
  'dependents',
  '_type',
])

function fieldText(value: unknown): string {
  if (value === null || value === undefined || value === '') return ''
  if (Array.isArray(value)) return value.map(fieldText).filter(Boolean).join(', ')
  if (typeof value === 'object') return JSON.stringify(value, null, 2)
  return String(value)
}

function firstField(issue: BeadsIssueDetail | null, fields: string[]): string {
  if (!issue) return ''
  for (const field of fields) {
    const text = fieldText(issue[field])
    if (text) return text
  }
  return ''
}

function renderLongField(label: string, value: unknown) {
  const text = fieldText(value)
  if (!text) return null
  return (
    <section className="bead-detail-section">
      <h3>{label}</h3>
      <pre>{text}</pre>
    </section>
  )
}

function commentBody(comment: BeadsComment): string {
  return fieldText(comment.body || comment.comment || comment.text || comment.content)
}

function commentAuthor(comment: BeadsComment): string {
  return fieldText(comment.author || comment.created_by || comment.user) || 'unknown'
}

function commentTime(comment: BeadsComment): string {
  return fieldText(comment.created_at || comment.created || comment.timestamp)
}

export default function IssueDetailModal({ projectPath, issue, onClose, onIssueUpdated }: IssueDetailModalProps) {
  const [commentDraft, setCommentDraft] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [submitError, setSubmitError] = useState<string | null>(null)
  const detail = useIssueDetail(projectPath, issue.id)
  const comments = useIssueComments(projectPath, issue.id)
  const activeIssue = (detail.issue || issue) as BeadsIssueDetail

  const metadata = [
    ['Status', firstField(activeIssue, ['status'])],
    ['Type', firstField(activeIssue, ['type', 'issue_type'])],
    ['Priority', firstField(activeIssue, ['priority'])],
    ['Assignee', firstField(activeIssue, ['assignee', 'owner'])],
    ['Created', firstField(activeIssue, ['created', 'created_at'])],
    ['Updated', firstField(activeIssue, ['updated', 'updated_at'])],
  ].filter(([, value]) => value)

  const extraFields = Object.entries(activeIssue)
    .filter(([key, value]) => !COMMON_FIELDS.has(key) && fieldText(value))
    .sort(([a], [b]) => a.localeCompare(b))

  const submitComment = async () => {
    const trimmed = commentDraft.trim()
    if (!trimmed || submitting) return
    setSubmitting(true)
    setSubmitError(null)
    try {
      const result = await addIssueComment(projectPath, issue.id, trimmed)
      if (!result.success) {
        setSubmitError(result.error?.message || 'Failed to add comment')
        return
      }
      setCommentDraft('')
      comments.refresh()
      detail.refresh()
      onIssueUpdated?.()
    } catch (error) {
      setSubmitError(error instanceof Error ? error.message : 'Network error')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="bead-detail-backdrop" role="presentation" onMouseDown={(event) => {
      if (event.target === event.currentTarget) onClose()
    }}>
      <div className="bead-detail-modal" role="dialog" aria-modal="true" aria-labelledby="bead-detail-title">
        <header className="bead-detail-header">
          <div>
            <div className="bead-detail-id">{activeIssue.id}</div>
            <h2 id="bead-detail-title">{activeIssue.title}</h2>
          </div>
          <button className="bead-detail-close" type="button" onClick={onClose} aria-label="Close bead detail">x</button>
        </header>

        {detail.error && <div className="bead-detail-error">{detail.error}</div>}

        <div className="bead-detail-body">
          <section className="bead-detail-section">
            <h3>Overview</h3>
            <dl className="bead-detail-meta">
              {metadata.map(([label, value]) => (
                <div key={label}>
                  <dt>{label}</dt>
                  <dd>{value}</dd>
                </div>
              ))}
            </dl>
            {Array.isArray(activeIssue.labels) && activeIssue.labels.length > 0 && (
              <div className="bead-detail-labels">
                {activeIssue.labels.map(label => <span key={label}>{label}</span>)}
              </div>
            )}
          </section>

          {detail.loading && <div className="bead-detail-loading">Loading detail...</div>}
          {renderLongField('Description', activeIssue.description)}
          {renderLongField('Design', activeIssue.design)}
          {renderLongField('Acceptance', activeIssue.acceptance || activeIssue.acceptance_criteria)}
          {renderLongField('Notes', activeIssue.notes)}
          {renderLongField('Dependencies', activeIssue.dependencies)}
          {renderLongField('Dependents', activeIssue.dependents)}

          {extraFields.length > 0 && (
            <section className="bead-detail-section">
              <h3>Other Fields</h3>
              <dl className="bead-detail-meta bead-detail-extra">
                {extraFields.map(([key, value]) => (
                  <div key={key}>
                    <dt>{key}</dt>
                    <dd><pre>{fieldText(value)}</pre></dd>
                  </div>
                ))}
              </dl>
            </section>
          )}

          <section className="bead-detail-section">
            <h3>Comments</h3>
            {comments.error && <div className="bead-detail-error">{comments.error}</div>}
            {comments.loading ? (
              <div className="bead-detail-loading">Loading comments...</div>
            ) : comments.comments.length === 0 ? (
              <div className="bead-detail-empty">No comments yet.</div>
            ) : (
              <div className="bead-comments-list">
                {comments.comments.map((comment, index) => (
                  <article className="bead-comment" key={fieldText(comment.id) || index}>
                    <header>
                      <span>{commentAuthor(comment)}</span>
                      {commentTime(comment) && <time>{commentTime(comment)}</time>}
                    </header>
                    <pre>{commentBody(comment)}</pre>
                  </article>
                ))}
              </div>
            )}

            <div className="bead-comment-form">
              <textarea
                value={commentDraft}
                onChange={(event) => setCommentDraft(event.target.value)}
                placeholder="Add comment"
                rows={4}
              />
              {submitError && <div className="bead-detail-error">{submitError}</div>}
              <button
                className="beads-refresh-btn"
                type="button"
                disabled={submitting || commentDraft.trim().length === 0}
                onClick={() => void submitComment()}
              >
                {submitting ? 'Adding...' : 'Add Comment'}
              </button>
            </div>
          </section>
        </div>
      </div>
    </div>
  )
}
