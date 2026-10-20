import { createRouter } from '@tanstack/react-router'
import { routeTree } from './routeTree.gen'

export function newRouter() {
  return createRouter({ routeTree })
}

declare module '@tanstack/react-router' {
  interface Register {
    router: ReturnType<typeof newRouter>
  }
}
