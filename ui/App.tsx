import {
  Badge,
  ButtonGroup,
  HStack,
  Icon,
  IconButton,
  List,
  ListItem,
  Skeleton,
  Text,
  TopNav,
  TopNavHeading,
  TopNavItem,
  TreeList,
  TreeListItemData,
  useImperativeAlertDialog,
  useImperativeDialog,
  useToast,
  VStack,
} from '@astryxdesign/core'
import { Play, Plus, RotateCcw, Square, SquarePen } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'

import Logo from './assets/icon.svg?react'
import ServiceEditor from './ServiceEditor.tsx'

export enum StartType {
  AutoStart,
  AutoStartDelayed,
  Manual,
  Disabled,
}

interface ProcessTree {
  pid: number
  exe: string
  children?: ProcessTree[]
}

interface Service {
  name: string
  start_type: StartType
  description?: string
  status: 'Stopped' | 'Starting' | 'Stopping' | 'Running' | 'Continuing' | 'Pausing' | 'Paused'
  // set by client
  processes?: ProcessTree
}

const statusToAction = {
  Stopped: 'Start',
  Starting: 'Stop',
  Stopping: 'Start',
  Running: 'Stop',
  Continuing: 'Stop',
  Pausing: 'Start',
  Paused: 'Start',
} as const
const actionToIcon = { Start: Play, Stop: Square } as const

export default function App() {
  const toast = useToast()
  const dialog = useImperativeDialog({ purpose: 'form' })
  const alert = useImperativeAlertDialog()
  const showAlert = (error: string) =>
    alert.show({
      title: 'Error',
      description: error,
      actionLabel: 'OK',
      actionVariant: 'secondary',
      onAction: alert.hide,
    })
  const [services, setServices] = useState<Service[] | undefined>(undefined)
  const [reconnect, setReconnect] = useState({})
  const ws = useRef<WebSocket>(null)
  useEffect(() => {
    const { protocol, host, pathname } = location
    const socket = new WebSocket(
      `${protocol === 'https:' ? 'wss:' : 'ws:'}//${host}${pathname}api/services`,
    )
    socket.onclose = evt =>
      evt.code !== 1000 && toast({ body: `WebSocket closed: ${evt.code}`, type: 'error' })
    socket.onerror = () => toast({ body: 'A WebSocket error occurred', type: 'error' })
    socket.onmessage = evt => {
      const data = JSON.parse(evt.data)
      if (data instanceof String) toast({ body: data, type: 'error' })
      else if (data instanceof Array) setServices(data)
      else
        setServices(srvs =>
          srvs?.map(srv => (srv.name === data.name ? { ...srv, status: data.status } : srv)),
        )
    }
    ws.current = socket
    return () => socket.close(1000)
  }, [toast, reconnect])

  return (
    <>
      <TopNav
        className="sticky top-0 z-1 bg-zinc-100 landscape:px-[calc(50vw-50vh)] dark:bg-zinc-800"
        heading={<TopNavHeading heading="Windows Service Manager" logo={<Icon icon={Logo} />} />}
        endContent={
          <TopNavItem
            label="New"
            icon={<Icon icon={Plus} size="sm" />}
            onClick={() =>
              dialog.show(
                <ServiceEditor
                  submit={async svc => {
                    if (
                      (await xfetch('/api/services', showAlert, {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify(svc),
                      })) != null
                    ) {
                      setReconnect({})
                      dialog.hide()
                    }
                  }}
                  close={dialog.hide}
                />,
              )
            }
          />
        }
      />
      <div className="landscape:px-[calc(50vw-50vh)]">
        {services ? (
          <List className="my-2" hasDividers>
            {services.map(({ name, start_type, description, status, processes }, i) => (
              <div key={name}>
                <ListItem
                  label={
                    <HStack gap={2}>
                      <Text maxLines={1}>{name}</Text>
                      <Badge label={StartType[start_type]} />
                      <Badge
                        variant={
                          (
                            {
                              Stopped: 'red',
                              Starting: 'blue',
                              Stopping: 'orange',
                              Running: 'green',
                              Continuing: 'blue',
                              Pausing: 'orange',
                              Paused: 'teal',
                            } as const
                          )[status]
                        }
                        label={status}
                      />
                    </HStack>
                  }
                  description={description}
                  startContent={
                    <IconButton
                      label="Collapse"
                      icon={<Icon icon={processes ? 'chevronDown' : 'chevronRight'} />}
                      variant="ghost"
                      isDisabled={status === 'Stopped' && !processes}
                      clickAction={async () => {
                        if (processes) {
                          setServices(srvs =>
                            srvs?.map((srv, j) =>
                              j === i ? { ...srv, processes: undefined } : srv,
                            ),
                          )
                        } else {
                          const json = await xfetch(
                            `/api/services/${encodeURIComponent(name)}/processes`,
                            showAlert,
                          )
                          if (json != null)
                            setServices(srvs =>
                              srvs?.map((srv, j) => (j === i ? { ...srv, processes: json } : srv)),
                            )
                        }
                      }}
                    />
                  }
                  endContent={
                    <ButtonGroup label="Actions" size="sm">
                      <IconButton
                        label={statusToAction[status]}
                        tooltip={statusToAction[status]}
                        icon={<Icon icon={actionToIcon[statusToAction[status]]} size="sm" />}
                        variant="ghost"
                        isDisabled={['Starting', 'Stopping', 'Continuing', 'Pausing'].includes(
                          status,
                        )}
                        clickAction={() =>
                          xfetch(
                            `/api/services/${encodeURIComponent(name)}/${status === 'Running' ? 'stop' : 'start'}`,
                            showAlert,
                            { method: 'PUT' },
                          )
                        }
                      />
                      <IconButton
                        label="Restart"
                        tooltip="Restart"
                        icon={<Icon icon={RotateCcw} size="sm" />}
                        variant="ghost"
                        isDisabled={[
                          'Stopped',
                          'Starting',
                          'Stopping',
                          'Continuing',
                          'Pausing',
                        ].includes(status)}
                        clickAction={() =>
                          xfetch(`/api/services/${encodeURIComponent(name)}/restart`, showAlert, {
                            method: 'PUT',
                          })
                        }
                      />
                      <IconButton
                        label="Edit"
                        tooltip="Edit"
                        icon={<Icon icon={SquarePen} size="sm" />}
                        variant="ghost"
                        clickAction={async () => {
                          const cfg = await xfetch(
                            `/api/services/${encodeURIComponent(name)}/config`,
                            showAlert,
                          )
                          if (cfg != null)
                            dialog.show(
                              <ServiceEditor
                                cfg={{ ...cfg, name }}
                                submit={async svc => {
                                  if (
                                    (await xfetch(
                                      `/api/services/${encodeURIComponent(name)}`,
                                      showAlert,
                                      {
                                        method: 'POST',
                                        headers: { 'Content-Type': 'application/json' },
                                        body: JSON.stringify(svc),
                                      },
                                    )) != null
                                  ) {
                                    setReconnect({})
                                    dialog.hide()
                                  }
                                }}
                                remove={async () => {
                                  if (
                                    (await xfetch(
                                      `/api/services/${encodeURIComponent(name)}`,
                                      showAlert,
                                      { method: 'DELETE' },
                                    )) != null
                                  ) {
                                    setReconnect({})
                                    dialog.hide()
                                  }
                                }}
                                close={dialog.hide}
                              />,
                            )
                        }}
                      />
                    </ButtonGroup>
                  }
                />
                {processes && (
                  <TreeList className="m-2 mx-3" items={[toTreeListItemData(processes)]} />
                )}
              </div>
            ))}
          </List>
        ) : (
          <VStack className="my-5" gap={3}>
            {['90%', '100%', '75%', '85%', '70%'].map((width, i) => (
              <Skeleton key={width} width={width} height={35} index={i} />
            ))}
          </VStack>
        )}
      </div>
      {dialog.isOpen && dialog.element}
      {alert.element}
    </>
  )
}

function toTreeListItemData({ pid, exe, children }: ProcessTree): TreeListItemData {
  return {
    id: pid.toString(),
    label: <Text maxLines={1}>{exe}</Text>,
    endContent: <Badge label={pid} />,
    children: children?.map(toTreeListItemData),
    isExpanded: true,
  }
}

async function xfetch(
  input: RequestInfo | URL,
  showError: (error: string) => void,
  init?: RequestInit,
) {
  try {
    const response = await fetch(input, init)
    const json = await response.json()
    if (json.message) {
      showError(json.message)
      return
    }
    return json
  } catch (error) {
    showError(String(error))
  }
}
