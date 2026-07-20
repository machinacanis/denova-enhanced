import { useEffect, useRef, useState, type CSSProperties } from 'react'
import { useTranslation } from 'react-i18next'
import { Copy, Minus, Square, X } from 'lucide-react'

/**
 * 桌面端（Wails）窗口控制按钮：最小化 / 最大化（还原） / 关闭。
 *
 * 采用官方推荐方式（https://wails.io/docs/guides/frameless/）：
 * 通过 `@wailsio/runtime` 的 `Window` 对象控制窗口。
 * 仅在 Wails WebView 环境渲染，Web 版（浏览器）返回 null，保持零影响。
 *
 * 设计为可嵌入应用顶栏（WorkbenchShell 的 topBar），与原生标题栏控件解耦。
 */

type WailsWindow = {
  Minimise: () => Promise<void>
  ToggleMaximise: () => Promise<void>
  Close: () => Promise<void>
  IsMaximised: () => Promise<boolean>
}

function hasWailsRuntime(): boolean {
  return typeof window !== 'undefined' && Boolean((window as unknown as { _wails?: unknown })._wails)
}

export function WindowControls() {
  const { t } = useTranslation()
  const [win, setWin] = useState<WailsWindow | null>(null)
  const [maximized, setMaximized] = useState(false)
  const cancelledRef = useRef(false)

  useEffect(() => {
    cancelledRef.current = false
    let timer: number | undefined
    let onReady: (() => void) | undefined

    const activate = () => {
      if (cancelledRef.current || !hasWailsRuntime()) return false
      import('@wailsio/runtime')
        .then((runtime) => {
          if (cancelledRef.current) return
          const wailsWindow = runtime.Window as unknown as WailsWindow
          setWin(wailsWindow)
          return wailsWindow.IsMaximised()
        })
        .then((isMax) => {
          if (!cancelledRef.current && typeof isMax === 'boolean') setMaximized(isMax)
        })
        .catch((error) => console.warn('[desktop] 加载 Wails 窗口运行时失败', error))
      return true
    }

    // Wails 运行时就绪后立即激活；否则轮询等待（运行时在页面加载后注入）。
    if (!activate()) {
      onReady = () => activate()
      window.addEventListener('wails:runtime-config-ready', onReady, { once: true })
      timer = window.setInterval(() => {
        if (activate() && timer !== undefined) window.clearInterval(timer)
      }, 100)
    }
    return () => {
      cancelledRef.current = true
      if (onReady) window.removeEventListener('wails:runtime-config-ready', onReady)
      if (timer !== undefined) window.clearInterval(timer)
    }
  }, [])

  if (!win) return null

  const handleMinimize = () => {
    win.Minimise().catch((e) => console.warn('[desktop] 最小化失败', e))
  }
  const handleToggleMaximize = () => {
    win.ToggleMaximise()
      .then(() => win.IsMaximised())
      .then((isMax) => setMaximized(isMax))
      .catch((e) => console.warn('[desktop] 切换最大化失败', e))
  }
  const handleClose = () => {
    win.Close().catch((e) => console.warn('[desktop] 关闭窗口失败', e))
  }

  const base = 'flex h-7 w-8 items-center justify-center rounded-[var(--nova-radius)] text-[var(--nova-text-muted)] transition-colors'

  return (
    <div
      className="ml-1 flex items-center gap-0.5"
      style={{ '--wails-draggable': 'no-drag' } as CSSProperties}
    >
      <button
        type="button"
        aria-label={t('common.windowMinimize')}
        title={t('common.windowMinimize')}
        onClick={handleMinimize}
        className={`${base} hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]`}
      >
        <Minus className="h-3.5 w-3.5" />
      </button>
      <button
        type="button"
        aria-label={maximized ? t('common.windowRestore') : t('common.windowMaximize')}
        title={maximized ? t('common.windowRestore') : t('common.windowMaximize')}
        onClick={handleToggleMaximize}
        className={`${base} hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]`}
      >
        {maximized ? <Copy className="h-3 w-3" /> : <Square className="h-3 w-3" />}
      </button>
      <button
        type="button"
        aria-label={t('common.windowClose')}
        title={t('common.windowClose')}
        onClick={handleClose}
        className={`${base} hover:bg-[#c42b1c] hover:text-white`}
      >
        <X className="h-4 w-4" />
      </button>
    </div>
  )
}
