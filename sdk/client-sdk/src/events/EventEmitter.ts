/**
 * Type-safe event emitter implementation
 */

type EventHandler<T = unknown> = (data: T) => void;
type EventMap = Record<string, unknown>;

/**
 * Type-safe event emitter
 */
export class EventEmitter<Events extends EventMap> {
  private handlers: Map<keyof Events, Set<EventHandler<Events[keyof Events]>>> =
    new Map();

  /**
   * Register an event handler
   */
  public on<K extends keyof Events>(
    event: K,
    handler: EventHandler<Events[K]>
  ): void {
    let eventHandlers = this.handlers.get(event);
    if (eventHandlers === undefined) {
      eventHandlers = new Set();
      this.handlers.set(event, eventHandlers);
    }
    eventHandlers.add(handler as EventHandler<Events[keyof Events]>);
  }

  /**
   * Unregister an event handler
   */
  public off<K extends keyof Events>(
    event: K,
    handler: EventHandler<Events[K]>
  ): void {
    const eventHandlers = this.handlers.get(event);
    if (eventHandlers !== undefined) {
      eventHandlers.delete(handler as EventHandler<Events[keyof Events]>);
      if (eventHandlers.size === 0) {
        this.handlers.delete(event);
      }
    }
  }

  /**
   * Register a one-time event handler
   */
  public once<K extends keyof Events>(
    event: K,
    handler: EventHandler<Events[K]>
  ): void {
    const onceHandler: EventHandler<Events[K]> = (data: Events[K]) => {
      this.off(event, onceHandler);
      handler(data);
    };
    this.on(event, onceHandler);
  }

  /**
   * Emit an event
   */
  protected emit<K extends keyof Events>(event: K, data: Events[K]): void {
    const eventHandlers = this.handlers.get(event);
    if (eventHandlers !== undefined) {
      // Create a copy to avoid issues if handlers modify the set
      const handlersCopy = [...eventHandlers];
      for (const handler of handlersCopy) {
        try {
          handler(data);
        } catch (error) {
          console.error(`Error in event handler for ${String(event)}:`, error);
        }
      }
    }
  }

  /**
   * Remove all handlers for an event or all events
   */
  public removeAllListeners(event?: keyof Events): void {
    if (event !== undefined) {
      this.handlers.delete(event);
    } else {
      this.handlers.clear();
    }
  }

  /**
   * Get the number of listeners for an event
   */
  public listenerCount(event: keyof Events): number {
    const eventHandlers = this.handlers.get(event);
    return eventHandlers !== undefined ? eventHandlers.size : 0;
  }
}
