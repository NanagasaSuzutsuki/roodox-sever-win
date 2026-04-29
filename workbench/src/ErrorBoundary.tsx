import React from "react";

import { safeRemoveItem } from "./workbench/persistence";

type Props = {
  children: React.ReactNode;
  storageKeys?: string[];
};

type State = {
  message: string | null;
  detail: string | null;
};

export default class ErrorBoundary extends React.Component<Props, State> {
  state: State = { message: null, detail: null };

  componentDidMount(): void {
    window.addEventListener("error", this.handleWindowError);
    window.addEventListener("unhandledrejection", this.handleUnhandledRejection);
  }

  componentWillUnmount(): void {
    window.removeEventListener("error", this.handleWindowError);
    window.removeEventListener("unhandledrejection", this.handleUnhandledRejection);
  }

  componentDidCatch(error: Error, info: React.ErrorInfo): void {
    this.setState({
      message: error.message || "Unknown UI error",
      detail: [error.stack, info.componentStack].filter(Boolean).join("\n\n") || null
    });
  }

  private handleWindowError = (event: ErrorEvent): void => {
    const message = event.error instanceof Error ? event.error.message : event.message;
    const detail = event.error instanceof Error ? event.error.stack ?? null : null;
    if (!message) return;
    this.setState({ message, detail });
  };

  private handleUnhandledRejection = (event: PromiseRejectionEvent): void => {
    const reason = event.reason;
    this.setState({
      message: reason instanceof Error ? reason.message : String(reason),
      detail: reason instanceof Error ? reason.stack ?? null : null
    });
  };

  private reloadApp = (): void => {
    window.location.reload();
  };

  private clearCacheAndReload = (): void => {
    for (const key of this.props.storageKeys ?? []) {
      safeRemoveItem(key);
    }
    window.location.reload();
  };

  render(): React.ReactNode {
    if (!this.state.message) {
      return this.props.children;
    }
    return (
      <div className="crash-shell">
        <section className="crash-card">
          <p className="eyebrow">UI recovery</p>
          <h1>界面遇到异常</h1>
          <p className="panel-subtitle">
            当前窗口没有继续渲染。先尝试直接重载；如果异常来自本地缓存，再清空本地 GUI 缓存后重载。
          </p>
          <div className="inline-warning">{this.state.message}</div>
          {this.state.detail ? <pre className="crash-detail">{this.state.detail}</pre> : null}
          <div className="action-row">
            <button className="primary" onClick={this.reloadApp}>重载界面</button>
            <button className="secondary" onClick={this.clearCacheAndReload}>清空本地缓存并重载</button>
          </div>
        </section>
      </div>
    );
  }
}
