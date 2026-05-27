import { useCallback, useEffect, useMemo, useRef, useState, type PointerEvent as ReactPointerEvent, type RefObject } from 'react'
import { ChevronDown, ChevronUp, GitBranch, Move, Plus, Trash2 } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import type { BranchSummary, PlotNode, Snapshot } from '../types'

interface BranchTimelineProps {
  snapshot: Snapshot | null
  branches: BranchSummary[]
  currentBranchId: string
  onSwitchBranch: (branchId: string) => void
  onCreateBranch: (turnId: string, title: string) => void
  onDeleteBranch: (branchId: string) => void
  expanded?: boolean
  fill?: boolean
  onExpandedChange?: (expanded: boolean) => void
}

interface TimelineRow {
  branchId: string
  branch?: BranchSummary
  nodes: PlotNode[]
  startColumn: number
  empty: boolean
  color: string
  colorSoft: string
}

interface PositionedNode {
  node: PlotNode
  row: number
  column: number
  x: number
  y: number
  color: string
  colorSoft: string
}

interface EmptyBranchMarker {
  branch: BranchSummary
  row: number
  column: number
  x: number
  y: number
  color: string
  from?: PositionedNode
}

interface GraphLayout {
  rows: TimelineRow[]
  positionedNodes: PositionedNode[]
  nodeById: Map<string, PositionedNode>
  connections: Array<{ from: PositionedNode; to: PositionedNode | EmptyBranchMarker; branchChanged: boolean; color: string; dashed?: boolean }>
  emptyBranches: EmptyBranchMarker[]
  width: number
  height: number
}

const COLUMN_WIDTH = 250
const LANE_HEIGHT = 82
const NODE_CARD_WIDTH = 188
const NODE_DOT_X = 18
const NODE_CENTER_Y = 24
const GRAPH_LEFT = 40
const GRAPH_TOP = 36
const GRAPH_RIGHT = 84
const GRAPH_BOTTOM = 34

const BRANCH_COLORS = [
  { color: '#5fa8ff', soft: 'rgba(95,168,255,0.14)' },
  { color: '#f05260', soft: 'rgba(240,82,96,0.14)' },
  { color: '#59c178', soft: 'rgba(89,193,120,0.14)' },
  { color: '#d8a84f', soft: 'rgba(216,168,79,0.15)' },
  { color: '#22c7d6', soft: 'rgba(34,199,214,0.14)' },
  { color: '#a78bfa', soft: 'rgba(167,139,250,0.14)' },
]

export function BranchTimeline({
  snapshot,
  branches,
  currentBranchId,
  onSwitchBranch,
  onCreateBranch,
  onDeleteBranch,
  expanded: controlledExpanded,
  fill = false,
  onExpandedChange,
}: BranchTimelineProps) {
  const [internalExpanded, setInternalExpanded] = useState(false)
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null)
  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [branchTitle, setBranchTitle] = useState('')
  const scrollRef = useRef<HTMLDivElement | null>(null)
  useDragScroll(scrollRef)

  const graphNodes = snapshot?.graph?.nodes || []
  const graphBranches = snapshot?.graph?.branches?.length ? snapshot.graph.branches : branches
  const selectedNode = graphNodes.find((node) => node.id === selectedNodeId) || null
  const expanded = controlledExpanded ?? internalExpanded
  const emptyBranchCount = graphBranches.filter((branch) => isEmptyBranch(branch, graphNodes)).length
  const layout = useMemo(() => buildGraphLayout(graphNodes, graphBranches), [graphBranches, graphNodes])

  const currentPositionedNode = useMemo(() => {
    const branchHead = graphBranches.find((branch) => branch.id === currentBranchId)?.head
    return layout.positionedNodes.find((item) => item.node.id === selectedNodeId) ||
      layout.positionedNodes.find((item) => item.node.id === branchHead) ||
      layout.positionedNodes.find((item) => item.node.current && item.node.branch_id === currentBranchId) ||
      layout.positionedNodes.find((item) => item.node.branch_id === currentBranchId && item.node.head) ||
      null
  }, [currentBranchId, graphBranches, layout.positionedNodes, selectedNodeId])

  useEffect(() => {
    if (!expanded || !currentPositionedNode) return
    const scroller = scrollRef.current
    if (!scroller) return
    window.requestAnimationFrame(() => {
      scrollElementTo(scroller, Math.max(0, currentPositionedNode.x - scroller.clientWidth * 0.35), Math.max(0, currentPositionedNode.y - scroller.clientHeight * 0.45), 'smooth')
    })
  }, [currentPositionedNode, expanded])

  const setExpanded = (nextExpanded: boolean) => {
    if (controlledExpanded === undefined) setInternalExpanded(nextExpanded)
    onExpandedChange?.(nextExpanded)
  }

  const selectNode = useCallback((node: PlotNode) => {
    setSelectedNodeId(node.id)
    if (node.branch_id !== currentBranchId) onSwitchBranch(node.branch_id)
  }, [currentBranchId, onSwitchBranch])

  const openCreateDialog = () => {
    if (!selectedNode) return
    setBranchTitle(`基于「${selectedNode.title}」的新剧情线`)
    setCreateDialogOpen(true)
  }

  const submitCreateBranch = () => {
    if (!selectedNode) return
    onCreateBranch(selectedNode.id, branchTitle.trim() || '新剧情线')
    setCreateDialogOpen(false)
    setBranchTitle('')
  }

  const deleteBranch = (branch: BranchSummary) => {
    const label = formatBranchName(branch)
    if (!window.confirm(`删除空剧情线「${label}」？`)) return
    onDeleteBranch(branch.id)
    if (selectedNode?.branch_id === branch.id) setSelectedNodeId(null)
  }

  return (
    <div className={`${fill ? 'h-full min-h-0' : expanded ? 'h-[min(430px,calc(100vh-96px))] min-h-[320px]' : 'h-[52px]'} border-t border-[#2f3540] bg-[#14171c] px-3 py-3 transition-[height] sm:px-4`}>
      <div className="flex items-center justify-between gap-2 text-xs text-[#858b96]">
        <button type="button" className="flex items-center gap-1.5 font-medium text-[#c3cad6] hover:text-[#edf2fa]" onClick={() => setExpanded(!expanded)}>
          <GitBranch className="h-3.5 w-3.5 text-[#5fa8ff]" />
          剧情路线图
          {expanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronUp className="h-3.5 w-3.5" />}
        </button>
        <div className="flex min-w-0 flex-1 items-center justify-end gap-2 overflow-hidden">
          <span className="truncate text-[#737d8d]">{graphNodes.length || snapshot?.turns?.length || 0} 个剧情节点</span>
          {emptyBranchCount > 0 && <Badge variant="outline" className="hidden border-[#4a3d2f] bg-[#282119] text-[#d5aa72] sm:inline-flex">{emptyBranchCount} 条空剧情线</Badge>}
          {selectedNode && (
            <Button variant="outline" size="xs" className="hidden gap-1.5 border-[#3a414d] bg-[#20242b] text-[#c3cbd7] hover:bg-[#252c38] sm:inline-flex" onClick={openCreateDialog}>
              <Plus className="h-3.5 w-3.5" />
              从选中节点创建
            </Button>
          )}
        </div>
      </div>

      {expanded && (
        <div className="mt-3 flex h-[calc(100%-40px)] min-h-0 flex-col overflow-hidden rounded-lg border border-[#2d3440] bg-[#10141b] shadow-[0_18px_42px_rgba(0,0,0,0.30),inset_0_1px_0_rgba(255,255,255,0.05)]">
          <div className="flex min-h-11 shrink-0 flex-wrap items-center justify-between gap-2 border-b border-[#29313c] bg-[#171c25]/95 px-3 py-2 backdrop-blur sm:px-4">
            <div className="flex min-w-0 flex-1 items-center gap-2 overflow-x-auto">
              {layout.rows.map((row) => (
                <button
                  key={row.branchId}
                  type="button"
                  className={`flex h-7 shrink-0 items-center gap-2 rounded-md border px-2 text-xs transition ${row.branchId === currentBranchId ? 'border-[#5fa8ff]/60 bg-[#1d3552] text-[#eaf4ff]' : 'border-[#303946] bg-[#111720] text-[#9aa4b5] hover:border-[#4b596c] hover:text-[#d6dce6]'}`}
                  onClick={() => onSwitchBranch(row.branchId)}
                  title={formatBranchName(row.branch)}
                >
                  <span className="h-2.5 w-2.5 rounded-full" style={{ background: row.color }} />
                  <span className="max-w-32 truncate">{formatBranchName(row.branch)}</span>
                  <span className="text-[#7e8898]">{row.nodes.length}</span>
                </button>
              ))}
              {layout.rows.length === 0 && <span className="text-xs text-[#858f9f]">还没有剧情路线。</span>}
            </div>
            <div className="flex shrink-0 items-center gap-2 text-[#8d96a7]">
              <span className="hidden items-center gap-1.5 text-xs sm:flex">
                <Move className="h-3.5 w-3.5" />
                拖动或滚轮浏览
              </span>
              <Button size="xs" variant="outline" className="gap-1.5 border-[#354051] bg-[#1a202b] text-[#c4ccd8] hover:bg-[#242b38]" disabled={!selectedNode} onClick={openCreateDialog}>
                <Plus className="h-3.5 w-3.5 text-[#d8b35f]" />
                创建剧情线
              </Button>
            </div>
          </div>

          <div ref={scrollRef} className="min-h-0 flex-1 cursor-grab overflow-auto bg-[#0f131a] active:cursor-grabbing" data-testid="branch-graph-scroll">
            <div
              data-testid="branch-graph-canvas"
              data-edge-count={layout.connections.length}
              className="relative min-w-max"
              style={{ width: layout.width, height: layout.height }}
            >
              <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_1px_1px,rgba(148,163,184,0.12)_1px,transparent_0)] [background-size:18px_18px]" />
              <svg className="pointer-events-none absolute inset-0 overflow-visible" width={layout.width} height={layout.height} aria-hidden="true">
                {layout.connections.map((connection) => (
                  <path
                    key={`${connection.from.node.id}-${'node' in connection.to ? connection.to.node.id : connection.to.branch.id}`}
                    d={connectionPath(connection.from, connection.to)}
                    fill="none"
                    stroke={connection.color}
                    strokeWidth={connection.branchChanged ? 2.6 : 2}
                    strokeDasharray={connection.dashed ? '4 6' : undefined}
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    opacity={0.9}
                  />
                ))}
              </svg>

              {layout.positionedNodes.map(({ node, x, y, color, colorSoft }) => (
                <button
                  key={node.id}
                  type="button"
                  className={`absolute z-10 flex h-[48px] cursor-grab items-start gap-2 rounded-lg border px-3 py-2 text-left shadow-[0_10px_22px_rgba(0,0,0,0.22)] backdrop-blur transition active:cursor-grabbing ${node.id === selectedNodeId ? 'border-[#f0cf8b] text-[#fff1ce] ring-2 ring-[#f0cf8b]/25' : node.current ? 'border-[#5fa8ff] text-[#eaf4ff]' : 'border-[#3a4656] text-[#c4ccd8] hover:border-[#74849a]'}`}
                  style={{ left: x, top: y, width: NODE_CARD_WIDTH, background: node.id === selectedNodeId ? 'rgba(64,48,28,0.96)' : colorSoft }}
                  onClick={() => selectNode(node)}
                  title={`${node.title}\n${node.summary}`}
                >
                  <span className="mt-1 h-2.5 w-2.5 shrink-0 rounded-full shadow-[0_0_14px_currentColor]" style={{ background: color, color }} />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-[12px] font-medium">{node.title}</span>
                    <span className="mt-0.5 block truncate text-[11px] text-[#8e98a8]">{node.summary || '剧情节点'}</span>
                  </span>
                  {node.head && <Badge variant="outline" className="h-5 border-[#425065] bg-[#1b2430] px-1.5 text-[10px] text-[#aeb8c8]">HEAD</Badge>}
                </button>
              ))}

              {layout.emptyBranches.map((empty) => (
                <div
                  key={empty.branch.id}
                  className="absolute z-10 flex h-[38px] cursor-grab items-center gap-2 rounded-lg border border-dashed px-3 text-xs text-[#b7beca] active:cursor-grabbing"
                  style={{ left: empty.x, top: empty.y + 5, width: NODE_CARD_WIDTH, borderColor: empty.color, background: 'rgba(21,25,34,0.88)' }}
                >
                  <span className="h-2.5 w-2.5 rounded-full" style={{ background: empty.color }} />
                  <span className="min-w-0 flex-1 truncate" title={formatBranchName(empty.branch)}>空剧情线</span>
                  <button
                    type="button"
                    className="rounded p-1 text-[#9d6673] hover:bg-[#3a2028] hover:text-[#ff9aaa]"
                    onClick={() => deleteBranch(empty.branch)}
                    aria-label={`删除空剧情线 ${formatBranchName(empty.branch)}`}
                    title="删除空剧情线"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </div>
              ))}

              {layout.rows.length === 0 && <span className="absolute left-6 top-6 text-xs text-[#858b96]">还没有剧情节点，输入第一句话开始。</span>}
            </div>
          </div>

          <div className="flex min-h-[64px] shrink-0 items-center justify-between gap-3 border-t border-[#29313c] bg-[#161b24] px-3 text-xs text-[#818b9b] sm:px-4">
            {selectedNode ? (
              <div className="min-w-0">
                <span className="text-[#d6dbe5]">已选节点：</span>
                <span className="truncate">{selectedNode.title}</span>
              </div>
            ) : (
              <span>点击剧情节点后，可从该节点创建新的剧情线。</span>
            )}
            <MiniMap layout={layout} scrollRef={scrollRef} />
            {selectedNode && (
              <Button size="xs" className="shrink-0 gap-1.5 bg-[#2d6fb8] hover:bg-[#347dca]" onClick={openCreateDialog}>
                <Plus className="h-3.5 w-3.5" />
                创建剧情线
              </Button>
            )}
          </div>
        </div>
      )}

      <Dialog open={createDialogOpen} onOpenChange={setCreateDialogOpen}>
        <DialogContent className="border-[#303238] bg-[#202329] text-[#d7dbe2]">
          <DialogHeader>
            <DialogTitle>从选中节点创建剧情线</DialogTitle>
            <DialogDescription className="text-[#9aa4b5]">
              {selectedNode ? `将从「${selectedNode.title}」分叉，创建后故事舞台会切换到新剧情线。` : ''}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Input className="border-[#3a3d45] bg-[#17191d] text-sm" value={branchTitle} onChange={(event) => setBranchTitle(event.target.value)} placeholder="剧情线名称" />
            {selectedNode?.summary && <div className="rounded-md border border-[#303743] bg-[#17191d] p-2 text-xs leading-5 text-[#aab2c0]">{selectedNode.summary}</div>}
          </div>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setCreateDialogOpen(false)}>取消</Button>
            <Button className="gap-1.5 bg-[#2d6fb8] hover:bg-[#347dca]" onClick={submitCreateBranch}>
              <Plus className="h-4 w-4" />
              创建并切换
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function MiniMap({ layout, scrollRef }: { layout: GraphLayout; scrollRef: RefObject<HTMLDivElement | null> }) {
  const [viewport, setViewport] = useState({ left: 0, top: 0, width: 100, height: 100 })
  const draggingRef = useRef(false)

  const updateViewport = useCallback(() => {
    const scroller = scrollRef.current
    if (!scroller || layout.width <= 0 || layout.height <= 0) return
    setViewport({
      left: (scroller.scrollLeft / layout.width) * 100,
      top: (scroller.scrollTop / layout.height) * 100,
      width: Math.min(100, (scroller.clientWidth / layout.width) * 100),
      height: Math.min(100, (scroller.clientHeight / layout.height) * 100),
    })
  }, [layout.height, layout.width, scrollRef])

  useEffect(() => {
    const scroller = scrollRef.current
    if (!scroller) return
    updateViewport()
    scroller.addEventListener('scroll', updateViewport, { passive: true })
    const observer = new ResizeObserver(updateViewport)
    observer.observe(scroller)
    return () => {
      scroller.removeEventListener('scroll', updateViewport)
      observer.disconnect()
    }
  }, [scrollRef, updateViewport])

  const moveTo = (event: ReactPointerEvent<HTMLDivElement>) => {
    const scroller = scrollRef.current
    if (!scroller) return
    const rect = event.currentTarget.getBoundingClientRect()
    const ratioX = Math.min(1, Math.max(0, (event.clientX - rect.left) / rect.width))
    const ratioY = Math.min(1, Math.max(0, (event.clientY - rect.top) / rect.height))
    scrollElementTo(
      scroller,
      Math.max(0, ratioX * layout.width - scroller.clientWidth / 2),
      Math.max(0, ratioY * layout.height - scroller.clientHeight / 2),
      draggingRef.current ? 'auto' : 'smooth',
    )
  }

  return (
    <div
      className="relative hidden h-12 min-w-[180px] flex-1 overflow-hidden rounded-lg border border-[#2b3340] bg-[#10151d] sm:block"
      onPointerDown={(event) => {
        draggingRef.current = true
        event.currentTarget.setPointerCapture(event.pointerId)
        moveTo(event)
      }}
      onPointerMove={(event) => {
        if (draggingRef.current) moveTo(event)
      }}
      onPointerUp={(event) => {
        draggingRef.current = false
        event.currentTarget.releasePointerCapture(event.pointerId)
      }}
      onPointerCancel={() => {
        draggingRef.current = false
      }}
      aria-label="剧情路线图缩略导航"
    >
      <svg className="absolute inset-0 h-full w-full" viewBox={`0 0 ${layout.width} ${layout.height}`} preserveAspectRatio="none" aria-hidden="true">
        {layout.connections.map((connection) => (
          <path
            key={`mini-${connection.from.node.id}-${'node' in connection.to ? connection.to.node.id : connection.to.branch.id}`}
            d={connectionPath(connection.from, connection.to)}
            fill="none"
            stroke={connection.color}
            strokeWidth={6}
            strokeLinecap="round"
            strokeLinejoin="round"
            opacity={connection.dashed ? 0.32 : 0.55}
          />
        ))}
        {layout.positionedNodes.map((item) => (
          <circle key={`mini-node-${item.node.id}`} cx={item.x + NODE_DOT_X} cy={item.y + NODE_CENTER_Y} r={5} fill={item.color} opacity={0.85} />
        ))}
      </svg>
      <div
        className="absolute rounded border border-[#a8c7ff] bg-[#89b4ff]/10 shadow-[0_0_18px_rgba(137,180,255,0.25)]"
        style={{
          left: `${viewport.left}%`,
          top: `${viewport.top}%`,
          width: `${viewport.width}%`,
          height: `${viewport.height}%`,
        }}
      />
    </div>
  )
}

function buildGraphLayout(nodes: PlotNode[], branches: BranchSummary[]): GraphLayout {
  const columnById = buildNodeColumns(nodes)
  const rowsByBranch = new Map<string, TimelineRow>()

  for (const [index, branch] of branches.entries()) {
    const palette = BRANCH_COLORS[index % BRANCH_COLORS.length]
    rowsByBranch.set(branch.id, {
      branchId: branch.id,
      branch,
      nodes: [],
      startColumn: Math.max(0, branch.from_event ? (columnById.get(branch.from_event) ?? 0) + 1 : 0),
      empty: isEmptyBranch(branch, nodes),
      color: palette.color,
      colorSoft: palette.soft,
    })
  }

  for (const node of nodes) {
    if (!rowsByBranch.has(node.branch_id)) {
      const palette = BRANCH_COLORS[rowsByBranch.size % BRANCH_COLORS.length]
      rowsByBranch.set(node.branch_id, { branchId: node.branch_id, nodes: [], startColumn: 0, empty: false, color: palette.color, colorSoft: palette.soft })
    }
    rowsByBranch.get(node.branch_id)?.nodes.push(node)
  }

  const rows = Array.from(rowsByBranch.values()).map((row) => ({
    ...row,
    nodes: row.nodes.sort((a, b) => {
      const columnDiff = (columnById.get(a.id) ?? 0) - (columnById.get(b.id) ?? 0)
      return columnDiff || a.id.localeCompare(b.id)
    }),
  }))

  const displayColumnById = new Map<string, number>()
  for (const row of rows) {
    let previousColumn = -1
    for (const node of row.nodes) {
      const column = Math.max(columnById.get(node.id) ?? 0, previousColumn + 1)
      displayColumnById.set(node.id, column)
      previousColumn = column
    }
  }

  let maxColumn = 0
  for (const node of nodes) maxColumn = Math.max(maxColumn, displayColumnById.get(node.id) ?? 0)
  for (const row of rows) maxColumn = Math.max(maxColumn, row.startColumn)

  const positionedNodes: PositionedNode[] = []
  const nodeById = new Map<string, PositionedNode>()
  rows.forEach((row, rowIndex) => {
    for (const node of row.nodes) {
      const column = displayColumnById.get(node.id) ?? 0
      const positioned = {
        node,
        row: rowIndex,
        column,
        x: GRAPH_LEFT + column * COLUMN_WIDTH,
        y: GRAPH_TOP + rowIndex * LANE_HEIGHT,
        color: row.color,
        colorSoft: row.colorSoft,
      }
      positionedNodes.push(positioned)
      nodeById.set(node.id, positioned)
    }
  })

  const connections: GraphLayout['connections'] = []
  const connectionKeys = new Set<string>()
  const addConnection = (from: PositionedNode | undefined, to: PositionedNode | EmptyBranchMarker | undefined, color: string, dashed = false) => {
    if (!from || !to) return
    const toId = 'node' in to ? to.node.id : to.branch.id
    const key = `${from.node.id}->${toId}`
    if (connectionKeys.has(key)) return
    connectionKeys.add(key)
    connections.push({ from, to, branchChanged: 'node' in to ? from.node.branch_id !== to.node.branch_id : true, color, dashed })
  }

  for (const positioned of positionedNodes) {
    if (!positioned.node.parent_id) continue
    addConnection(nodeById.get(positioned.node.parent_id), positioned, positioned.color)
  }
  for (const row of rows) {
    for (let index = 1; index < row.nodes.length; index += 1) {
      addConnection(nodeById.get(row.nodes[index - 1].id), nodeById.get(row.nodes[index].id), row.color)
    }
  }

  const emptyBranches = rows.flatMap((row, rowIndex) => {
    if (!row.branch || !row.empty) return []
    const column = row.startColumn
    return [{
      branch: row.branch,
      row: rowIndex,
      column,
      x: GRAPH_LEFT + column * COLUMN_WIDTH,
      y: GRAPH_TOP + rowIndex * LANE_HEIGHT,
      color: row.color,
      from: row.branch.from_event ? nodeById.get(row.branch.from_event) : undefined,
    }]
  })

  for (const empty of emptyBranches) {
    addConnection(empty.from, empty, empty.color, true)
  }

  return {
    rows,
    positionedNodes,
    nodeById,
    connections,
    emptyBranches,
    width: Math.max(900, GRAPH_LEFT + GRAPH_RIGHT + (maxColumn + 1) * COLUMN_WIDTH + NODE_CARD_WIDTH),
    height: Math.max(220, GRAPH_TOP + GRAPH_BOTTOM + rows.length * LANE_HEIGHT),
  }
}

function connectionPath(from: Pick<PositionedNode, 'x' | 'y'>, to: Pick<PositionedNode | EmptyBranchMarker, 'x' | 'y'>) {
  const startX = from.x + NODE_CARD_WIDTH
  const startY = from.y + NODE_CENTER_Y
  const endX = to.x
  const endY = to.y + NODE_CENTER_Y
  const curve = Math.max(52, Math.min(120, Math.abs(endX - startX) * 0.42))
  return `M ${startX} ${startY} C ${startX + curve} ${startY}, ${endX - curve} ${endY}, ${endX} ${endY}`
}

function buildNodeColumns(nodes: PlotNode[]) {
  const byId = new Map(nodes.map((node) => [node.id, node]))
  const columnById = new Map<string, number>()

  const getColumn = (nodeId: string, path = new Set<string>()): number => {
    const cached = columnById.get(nodeId)
    if (cached !== undefined) return cached
    if (path.has(nodeId)) return 0
    path.add(nodeId)
    const node = byId.get(nodeId)
    const column = node?.parent_id ? getColumn(node.parent_id, path) + 1 : 0
    path.delete(nodeId)
    columnById.set(nodeId, column)
    return column
  }

  for (const node of nodes) getColumn(node.id)
  return columnById
}

function isEmptyBranch(branch: BranchSummary, nodes: PlotNode[]) {
  return branch.id !== 'main' && branch.head === branch.from_event && !nodes.some((node) => node.branch_id === branch.id)
}

function formatBranchName(branch?: BranchSummary) {
  if (!branch) return '未知剧情线'
  if (branch.title?.trim()) return branch.title.trim()
  if (branch.id === 'main') return '主线'
  return branch.id
}

function scrollElementTo(element: HTMLElement, left: number, top: number, behavior: ScrollBehavior) {
  if (typeof element.scrollTo === 'function') {
    element.scrollTo({ left, top, behavior })
    return
  }
  element.scrollLeft = left
  element.scrollTop = top
}

function useDragScroll(ref: RefObject<HTMLElement | null>) {
  const dragRef = useRef<{ x: number; y: number; left: number; top: number; active: boolean; moved: boolean; suppressClick: boolean }>({
    x: 0,
    y: 0,
    left: 0,
    top: 0,
    active: false,
    moved: false,
    suppressClick: false,
  })

  useEffect(() => {
    const node = ref.current
    if (!node) return

    const onPointerDown = (event: PointerEvent) => {
      if ((event.target as HTMLElement).closest('input,textarea,select,[data-no-drag]')) return
      dragRef.current = { x: event.clientX, y: event.clientY, left: node.scrollLeft, top: node.scrollTop, active: true, moved: false, suppressClick: false }
      node.setPointerCapture(event.pointerId)
    }
    const onPointerMove = (event: PointerEvent) => {
      if (!dragRef.current.active) return
      const deltaX = event.clientX - dragRef.current.x
      const deltaY = event.clientY - dragRef.current.y
      if (!dragRef.current.moved && Math.hypot(deltaX, deltaY) > 4) {
        dragRef.current.moved = true
        dragRef.current.suppressClick = true
      }
      if (!dragRef.current.moved) return
      event.preventDefault()
      node.scrollLeft = dragRef.current.left - deltaX
      node.scrollTop = dragRef.current.top - deltaY
    }
    const onPointerUp = (event: PointerEvent) => {
      dragRef.current.active = false
      if (node.hasPointerCapture(event.pointerId)) node.releasePointerCapture(event.pointerId)
    }
    const onPointerCancel = () => {
      dragRef.current.active = false
    }

    const onClickCapture = (event: MouseEvent) => {
      if (!dragRef.current.suppressClick) return
      dragRef.current.suppressClick = false
      event.preventDefault()
      event.stopPropagation()
    }

    node.addEventListener('pointerdown', onPointerDown)
    node.addEventListener('pointermove', onPointerMove)
    node.addEventListener('pointerup', onPointerUp)
    node.addEventListener('pointercancel', onPointerCancel)
    node.addEventListener('click', onClickCapture, true)
    return () => {
      node.removeEventListener('pointerdown', onPointerDown)
      node.removeEventListener('pointermove', onPointerMove)
      node.removeEventListener('pointerup', onPointerUp)
      node.removeEventListener('pointercancel', onPointerCancel)
      node.removeEventListener('click', onClickCapture, true)
    }
  }, [ref])
}
