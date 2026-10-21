import { api } from './api'
import { Refused, Unauthenticated } from './session'

async function read<T>(call: Promise<{ data?: T; error?: { message?: string }; response: Response }>, name: string) {
  const { data, error, response } = await call
  if (response.status === 401) {
    throw new Unauthenticated()
  }
  if (!data) {
    throw response.status < 500 && error?.message ? new Refused(error.message) : new Error(name)
  }
  return data
}

export const operatorsQuery = { queryKey: ['operators'], queryFn: () => read(api.GET('/operators'), 'operators') }

export const rolesQuery = { queryKey: ['roles'], queryFn: () => read(api.GET('/roles'), 'roles') }
