// Beads Graph View - Interactive dependency graph visualization
// This component is part of the self-contained beads_module

import { useEffect, useRef, useState, useCallback } from 'react';
import { useBeads } from '../context';
import type {
  BeadsGraphNode,
  BeadsGraphLink,
  BeadsGraphViewProps,
} from '../types';
import {
  BEADS_STATUS_COLORS,
  BEADS_TYPE_ICONS,
  BEADS_PRIORITY_COLORS,
} from '../types';

// ============================================================================
// GRAPH RENDERING (Canvas-based for performance)
// ============================================================================

interface CanvasGraphProps {
  nodes: BeadsGraphNode[];
  links: BeadsGraphLink[];
  selectedNodeId: string | null;
  onNodeClick: (nodeId: string) => void;
  onNodeHover: (nodeId: string | null) => void;
  colorBy: 'status' | 'priority' | 'type';
}

interface NodePosition {
  x: number;
  y: number;
  vx: number;
  vy: number;
}

function CanvasGraph({
  nodes,
  links,
  selectedNodeId,
  onNodeClick,
  onNodeHover,
  colorBy,
}: CanvasGraphProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const animationRef = useRef<number>();
  const positionsRef = useRef<Map<string, NodePosition>>(new Map());
  const [hoveredNodeId, setHoveredNodeId] = useState<string | null>(null);
  const [dimensions, setDimensions] = useState({ width: 800, height: 600 });

  // Initialize positions
  useEffect(() => {
    const positions = positionsRef.current;
    nodes.forEach((node) => {
      if (!positions.has(node.id)) {
        positions.set(node.id, {
          x: Math.random() * dimensions.width,
          y: Math.random() * dimensions.height,
          vx: 0,
          vy: 0,
        });
      }
    });
    // Clean up removed nodes
    positions.forEach((_, id) => {
      if (!nodes.find((n) => n.id === id)) {
        positions.delete(id);
      }
    });
  }, [nodes, dimensions]);

  // Resize handler
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const resizeObserver = new ResizeObserver((entries) => {
      const entry = entries[0];
      if (entry) {
        setDimensions({
          width: entry.contentRect.width,
          height: entry.contentRect.height,
        });
      }
    });

    resizeObserver.observe(container);
    return () => resizeObserver.disconnect();
  }, []);

  // Get node color based on colorBy setting
  const getNodeColor = useCallback(
    (node: BeadsGraphNode): string => {
      switch (colorBy) {
        case 'status':
          return BEADS_STATUS_COLORS[node.status];
        case 'priority':
          return BEADS_PRIORITY_COLORS[node.priority] || BEADS_PRIORITY_COLORS[5];
        case 'type':
          // Use status colors but mapped to types
          const typeColors: Record<string, string> = {
            bug: '#f44336',
            feature: '#4CAF50',
            task: '#2196F3',
            epic: '#9C27B0',
            chore: '#FF9800',
          };
          return typeColors[node.type] || '#9E9E9E';
        default:
          return BEADS_STATUS_COLORS[node.status];
      }
    },
    [colorBy]
  );

  // Force simulation
  useEffect(() => {
    const positions = positionsRef.current;
    const canvas = canvasRef.current;
    if (!canvas) return;

    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    // Build adjacency for link lookup
    const linkMap = new Map<string, Set<string>>();
    links.forEach((link) => {
      if (!linkMap.has(link.source)) linkMap.set(link.source, new Set());
      if (!linkMap.has(link.target)) linkMap.set(link.target, new Set());
      linkMap.get(link.source)!.add(link.target);
      linkMap.get(link.target)!.add(link.source);
    });

    const simulate = () => {
      // Force simulation parameters
      const centerX = dimensions.width / 2;
      const centerY = dimensions.height / 2;
      const repulsion = 5000;
      const attraction = 0.01;
      const damping = 0.9;
      const centerPull = 0.01;

      // Apply forces
      nodes.forEach((node) => {
        const pos = positions.get(node.id);
        if (!pos) return;

        // Center pull
        pos.vx += (centerX - pos.x) * centerPull;
        pos.vy += (centerY - pos.y) * centerPull;

        // Repulsion from other nodes
        nodes.forEach((other) => {
          if (node.id === other.id) return;
          const otherPos = positions.get(other.id);
          if (!otherPos) return;

          const dx = pos.x - otherPos.x;
          const dy = pos.y - otherPos.y;
          const dist = Math.sqrt(dx * dx + dy * dy) || 1;
          const force = repulsion / (dist * dist);

          pos.vx += (dx / dist) * force;
          pos.vy += (dy / dist) * force;
        });
      });

      // Attraction along links
      links.forEach((link) => {
        const sourcePos = positions.get(link.source);
        const targetPos = positions.get(link.target);
        if (!sourcePos || !targetPos) return;

        const dx = targetPos.x - sourcePos.x;
        const dy = targetPos.y - sourcePos.y;

        sourcePos.vx += dx * attraction;
        sourcePos.vy += dy * attraction;
        targetPos.vx -= dx * attraction;
        targetPos.vy -= dy * attraction;
      });

      // Apply velocity and damping
      nodes.forEach((node) => {
        const pos = positions.get(node.id);
        if (!pos) return;

        pos.vx *= damping;
        pos.vy *= damping;
        pos.x += pos.vx;
        pos.y += pos.vy;

        // Boundary constraints
        const margin = 50;
        pos.x = Math.max(margin, Math.min(dimensions.width - margin, pos.x));
        pos.y = Math.max(margin, Math.min(dimensions.height - margin, pos.y));
      });

      // Draw
      ctx.clearRect(0, 0, dimensions.width, dimensions.height);

      // Draw links
      links.forEach((link) => {
        const sourcePos = positions.get(link.source);
        const targetPos = positions.get(link.target);
        if (!sourcePos || !targetPos) return;

        ctx.beginPath();
        ctx.moveTo(sourcePos.x, sourcePos.y);
        ctx.lineTo(targetPos.x, targetPos.y);

        // Color by link type
        if (link.type === 'blocks') {
          ctx.strokeStyle = 'rgba(244, 67, 54, 0.4)';
        } else if (link.type === 'depends') {
          ctx.strokeStyle = 'rgba(33, 150, 243, 0.4)';
        } else {
          ctx.strokeStyle = 'rgba(156, 39, 176, 0.3)';
        }
        ctx.lineWidth = link.type === 'blocks' ? 2 : 1;
        ctx.stroke();

        // Draw arrow
        const angle = Math.atan2(targetPos.y - sourcePos.y, targetPos.x - sourcePos.x);
        const arrowLength = 10;
        const arrowX = targetPos.x - Math.cos(angle) * 20;
        const arrowY = targetPos.y - Math.sin(angle) * 20;

        ctx.beginPath();
        ctx.moveTo(arrowX, arrowY);
        ctx.lineTo(
          arrowX - arrowLength * Math.cos(angle - Math.PI / 6),
          arrowY - arrowLength * Math.sin(angle - Math.PI / 6)
        );
        ctx.lineTo(
          arrowX - arrowLength * Math.cos(angle + Math.PI / 6),
          arrowY - arrowLength * Math.sin(angle + Math.PI / 6)
        );
        ctx.closePath();
        ctx.fillStyle = ctx.strokeStyle;
        ctx.fill();
      });

      // Draw nodes
      nodes.forEach((node) => {
        const pos = positions.get(node.id);
        if (!pos) return;

        const isSelected = node.id === selectedNodeId;
        const isHovered = node.id === hoveredNodeId;
        const radius = isSelected || isHovered ? 16 : 12;

        // Node circle
        ctx.beginPath();
        ctx.arc(pos.x, pos.y, radius, 0, Math.PI * 2);
        ctx.fillStyle = getNodeColor(node);
        ctx.fill();

        // Border
        if (isSelected) {
          ctx.strokeStyle = '#ffffff';
          ctx.lineWidth = 3;
          ctx.stroke();
        } else if (isHovered) {
          ctx.strokeStyle = '#ffffff';
          ctx.lineWidth = 2;
          ctx.stroke();
        }

        // Label
        ctx.font = '11px monospace';
        ctx.fillStyle = '#ffffff';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'top';

        const label =
          node.name.length > 20 ? node.name.substring(0, 17) + '...' : node.name;
        ctx.fillText(label, pos.x, pos.y + radius + 4);
      });

      animationRef.current = requestAnimationFrame(simulate);
    };

    simulate();

    return () => {
      if (animationRef.current) {
        cancelAnimationFrame(animationRef.current);
      }
    };
  }, [nodes, links, dimensions, selectedNodeId, hoveredNodeId, getNodeColor]);

  // Mouse interaction
  const handleMouseMove = useCallback(
    (e: React.MouseEvent<HTMLCanvasElement>) => {
      const canvas = canvasRef.current;
      if (!canvas) return;

      const rect = canvas.getBoundingClientRect();
      const x = e.clientX - rect.left;
      const y = e.clientY - rect.top;

      const positions = positionsRef.current;
      let foundNode: string | null = null;

      nodes.forEach((node) => {
        const pos = positions.get(node.id);
        if (!pos) return;

        const dx = x - pos.x;
        const dy = y - pos.y;
        const dist = Math.sqrt(dx * dx + dy * dy);

        if (dist < 16) {
          foundNode = node.id;
        }
      });

      if (foundNode !== hoveredNodeId) {
        setHoveredNodeId(foundNode);
        onNodeHover(foundNode);
      }
    },
    [nodes, hoveredNodeId, onNodeHover]
  );

  const handleClick = useCallback(
    (e: React.MouseEvent<HTMLCanvasElement>) => {
      const canvas = canvasRef.current;
      if (!canvas) return;

      const rect = canvas.getBoundingClientRect();
      const x = e.clientX - rect.left;
      const y = e.clientY - rect.top;

      const positions = positionsRef.current;

      nodes.forEach((node) => {
        const pos = positions.get(node.id);
        if (!pos) return;

        const dx = x - pos.x;
        const dy = y - pos.y;
        const dist = Math.sqrt(dx * dx + dy * dy);

        if (dist < 16) {
          onNodeClick(node.id);
        }
      });
    },
    [nodes, onNodeClick]
  );

  return (
    <div ref={containerRef} className="beads-graph-canvas-container">
      <canvas
        ref={canvasRef}
        width={dimensions.width}
        height={dimensions.height}
        onMouseMove={handleMouseMove}
        onClick={handleClick}
        style={{ cursor: hoveredNodeId ? 'pointer' : 'default' }}
      />
    </div>
  );
}

// ============================================================================
// MAIN COMPONENT
// ============================================================================

export function BeadsGraphView({ projectPath, onIssueSelect }: BeadsGraphViewProps) {
  const {
    issues,
    graphData,
    isLoading,
    error,
    fetchIssues,
    selectIssue,
    viewState,
  } = useBeads();

  const [colorBy, setColorBy] = useState<'status' | 'priority' | 'type'>('status');
  const [hoveredNode, setHoveredNode] = useState<BeadsGraphNode | null>(null);

  // Fetch data on mount and when project changes
  useEffect(() => {
    fetchIssues(projectPath);
  }, [projectPath, fetchIssues]);

  const handleNodeClick = useCallback(
    (nodeId: string) => {
      selectIssue(nodeId);
      onIssueSelect?.(nodeId);
    },
    [selectIssue, onIssueSelect]
  );

  const handleNodeHover = useCallback(
    (nodeId: string | null) => {
      if (nodeId && graphData) {
        const node = graphData.nodes.find((n) => n.id === nodeId);
        setHoveredNode(node || null);
      } else {
        setHoveredNode(null);
      }
    },
    [graphData]
  );

  const handleRefresh = useCallback(() => {
    fetchIssues(projectPath);
  }, [fetchIssues, projectPath]);

  // Loading state
  if (isLoading && issues.length === 0) {
    return (
      <div className="beads-view beads-graph-view">
        <div className="beads-loading">
          <div className="beads-spinner" />
          <span>Loading graph data...</span>
        </div>
      </div>
    );
  }

  // Error state
  if (error && issues.length === 0) {
    return (
      <div className="beads-view beads-graph-view">
        <div className="beads-error">
          <span className="beads-error-icon">⚠️</span>
          <span className="beads-error-message">{error.message}</span>
          {error.details && (
            <span className="beads-error-details">{error.details}</span>
          )}
          <button className="beads-btn beads-btn-primary" onClick={handleRefresh}>
            Retry
          </button>
        </div>
      </div>
    );
  }

  // Empty state
  if (!graphData || graphData.nodes.length === 0) {
    return (
      <div className="beads-view beads-graph-view">
        <div className="beads-empty">
          <span className="beads-empty-icon">📊</span>
          <span className="beads-empty-message">No issues found</span>
          <span className="beads-empty-hint">
            Make sure the project has a .beads/issues.jsonl file
          </span>
        </div>
      </div>
    );
  }

  return (
    <div className="beads-view beads-graph-view">
      {/* Toolbar */}
      <div className="beads-toolbar">
        <div className="beads-toolbar-left">
          <span className="beads-toolbar-label">
            {graphData.nodes.length} issues, {graphData.links.length} links
          </span>
        </div>
        <div className="beads-toolbar-right">
          <label className="beads-toolbar-control">
            <span>Color by:</span>
            <select
              value={colorBy}
              onChange={(e) => setColorBy(e.target.value as typeof colorBy)}
              className="beads-select"
            >
              <option value="status">Status</option>
              <option value="priority">Priority</option>
              <option value="type">Type</option>
            </select>
          </label>
          <button
            className="beads-btn beads-btn-icon"
            onClick={handleRefresh}
            disabled={isLoading}
            title="Refresh"
          >
            {isLoading ? '⏳' : '🔄'}
          </button>
        </div>
      </div>

      {/* Graph */}
      <div className="beads-graph-container">
        <CanvasGraph
          nodes={graphData.nodes}
          links={graphData.links}
          selectedNodeId={viewState.selectedIssueId}
          onNodeClick={handleNodeClick}
          onNodeHover={handleNodeHover}
          colorBy={colorBy}
        />

        {/* Legend */}
        <div className="beads-graph-legend">
          <div className="beads-legend-title">Legend</div>
          {colorBy === 'status' && (
            <div className="beads-legend-items">
              {Object.entries(BEADS_STATUS_COLORS).map(([status, color]) => (
                <div key={status} className="beads-legend-item">
                  <span
                    className="beads-legend-color"
                    style={{ backgroundColor: color }}
                  />
                  <span className="beads-legend-label">{status}</span>
                </div>
              ))}
            </div>
          )}
          {colorBy === 'priority' && (
            <div className="beads-legend-items">
              {[1, 2, 3, 4, 5].map((p) => (
                <div key={p} className="beads-legend-item">
                  <span
                    className="beads-legend-color"
                    style={{ backgroundColor: BEADS_PRIORITY_COLORS[p] }}
                  />
                  <span className="beads-legend-label">P{p}</span>
                </div>
              ))}
            </div>
          )}
          <div className="beads-legend-links">
            <div className="beads-legend-item">
              <span className="beads-legend-line beads-legend-line-blocks" />
              <span className="beads-legend-label">Blocks</span>
            </div>
            <div className="beads-legend-item">
              <span className="beads-legend-line beads-legend-line-depends" />
              <span className="beads-legend-label">Depends</span>
            </div>
          </div>
        </div>

        {/* Hover tooltip */}
        {hoveredNode && (
          <div className="beads-graph-tooltip">
            <div className="beads-tooltip-header">
              <span className="beads-tooltip-icon">
                {BEADS_TYPE_ICONS[hoveredNode.type]}
              </span>
              <span className="beads-tooltip-id">{hoveredNode.id}</span>
            </div>
            <div className="beads-tooltip-title">{hoveredNode.name}</div>
            <div className="beads-tooltip-meta">
              <span
                className="beads-tooltip-status"
                style={{ color: BEADS_STATUS_COLORS[hoveredNode.status] }}
              >
                {hoveredNode.status}
              </span>
              <span className="beads-tooltip-priority">P{hoveredNode.priority}</span>
            </div>
            {hoveredNode.labels.length > 0 && (
              <div className="beads-tooltip-labels">
                {hoveredNode.labels.slice(0, 3).map((label) => (
                  <span key={label} className="beads-tooltip-label">
                    {label}
                  </span>
                ))}
                {hoveredNode.labels.length > 3 && (
                  <span className="beads-tooltip-label">
                    +{hoveredNode.labels.length - 3}
                  </span>
                )}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

export default BeadsGraphView;
