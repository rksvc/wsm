import {
  Badge,
  Button,
  DialogHeader,
  FieldLabel,
  FormLayout,
  HStack,
  Icon,
  Layout,
  LayoutContent,
  LayoutFooter,
  Selector,
  Stack,
  StackItem,
  Text,
  TextArea,
  TextInput,
  useImperativeAlertDialog,
} from '@astryxdesign/core'
import { useState } from 'react'

import { StartType } from './App.tsx'

interface Config {
  name: string
  description: string
  exe: string
  flags: string[]
  dir: string
  stdin: string
  stdout: string
  stderr: string
  env: string[]
  dependencies: string[]
  start_type: StartType
}

export default function ServiceEditor({
  cfg,
  submit,
  remove,
  close,
}: {
  cfg?: Config
  submit: (c: Config) => Promise<void>
  remove?: () => Promise<void>
  close: () => void
}) {
  const [name, setName] = useState(cfg?.name ?? '')
  const [desc, setDesc] = useState(cfg?.description ?? '')
  const [exe, setExe] = useState(cfg?.exe ?? '')
  const [flag, setFlag] = useState('')
  const [flags, setFlags] = useState<string[]>(cfg?.flags ?? [])
  const [dir, setDir] = useState(cfg?.dir ?? '')
  const [stdin, setStdin] = useState(cfg?.stdin ?? '')
  const [stdout, setStdout] = useState(cfg?.stdout ?? '')
  const [stderr, setStderr] = useState(cfg?.stderr ?? '')
  const [env, setEnv] = useState(cfg?.env.join('\n') ?? '')
  const [deps, setDeps] = useState(cfg?.dependencies.join('\n') ?? '')
  const [startType, setStartType] = useState<keyof typeof StartType>(
    cfg == null ? 'AutoStart' : (StartType[cfg.start_type] as keyof typeof StartType),
  )

  const alert = useImperativeAlertDialog()
  const [nameError, setNameError] = useState<undefined | true>(undefined)
  const [exeError, setExeError] = useState<undefined | true>(undefined)
  const [isLoading, setIsLoading] = useState(false)

  return (
    <Layout
      header={<DialogHeader title={`${cfg ? 'Edit' : 'New'} service`} onOpenChange={close} />}
      content={
        <LayoutContent>
          <FormLayout>
            <TextInput
              label="Name"
              value={name}
              onChange={name => {
                setName(name)
                setNameError(undefined)
              }}
              disabledMessage="The service name is immutable"
              hasAutoFocus={!cfg}
              isDisabled={Boolean(cfg)}
              status={nameError && { type: 'error', message: 'Name is required' }}
              isRequired
              hasClear
            />
            <TextInput
              label="Path"
              value={exe}
              onChange={exe => {
                setExe(exe)
                setExeError(undefined)
              }}
              hasAutoFocus={Boolean(cfg)}
              status={exeError && { type: 'error', message: 'Path is required' }}
              isRequired
              hasClear
            />
            <Stack gap={1}>
              <HStack gap={1} wrap="wrap">
                <FieldLabel label="Arguments:" inputID="argument" />
                {flags.length ? (
                  flags.map((flag, i) => (
                    <Badge
                      key={`${flag}${i}`}
                      className="max-w-full"
                      icon={
                        <Icon
                          className="cursor-pointer"
                          icon="close"
                          size="sm"
                          onClick={() => setFlags(flags => flags.toSpliced(i, 1))}
                        />
                      }
                      label={<Text maxLines={1}>{flag}</Text>}
                    />
                  ))
                ) : (
                  <Text type="supporting">{'<empty>'}</Text>
                )}
              </HStack>
              <HStack gap={1}>
                <StackItem size="fill">
                  <TextInput
                    id="argument"
                    label="Argument"
                    isLabelHidden
                    value={flag}
                    onChange={setFlag}
                    hasClear
                  />
                </StackItem>
                <Button
                  label="Add"
                  onClick={() => {
                    if (!flag) return
                    setFlags(flags => [...flags, flag])
                    setFlag('')
                  }}
                />
              </HStack>
            </Stack>
            <TextInput label="Startup directory" value={dir} onChange={setDir} hasClear />
            <TextInput label="Description" value={desc} onChange={setDesc} hasClear />
            <TextInput label="Input (stdin)" value={stdin} onChange={setStdin} hasClear />
            <TextInput label="Output (stdout)" value={stdout} onChange={setStdout} hasClear />
            <TextInput label="Error (stderr)" value={stderr} onChange={setStderr} hasClear />
            <TextArea label="Environment variables" value={env} onChange={setEnv} />
            <TextArea
              label="Dependencies"
              description="The service depend on the following system components"
              value={deps}
              onChange={setDeps}
            />
            <Selector
              label="Startup type"
              options={[
                { value: 'AutoStart', label: 'Automatic' },
                { value: 'AutoStartDelayed', label: 'Automatic (Delayed Start)' },
                { value: 'Manual', label: 'Manual' },
                { value: 'Disabled', label: 'Disabled' },
              ]}
              value={startType}
              onChange={startType => setStartType(startType as keyof typeof StartType)}
            />
          </FormLayout>
        </LayoutContent>
      }
      footer={
        <LayoutFooter>
          <HStack gap={2} hAlign="end">
            {cfg && remove && (
              <Button
                label="Delete"
                variant="destructive"
                isLoading={isLoading}
                clickAction={() =>
                  alert.show({
                    title: `Delete ${cfg.name}`,
                    description: `Are you sure you want to delete ${cfg.name}?`,
                    actionLabel: 'Delete',
                    onAction: async () => {
                      setIsLoading(true)
                      alert.hide()
                      await remove()
                      setIsLoading(false)
                    },
                  })
                }
              />
            )}
            <Button
              label="Submit"
              variant="primary"
              isLoading={isLoading}
              clickAction={async () => {
                if (!name) return setNameError(true)
                if (!exe) return setExeError(true)
                await submit({
                  name,
                  exe,
                  flags,
                  dir,
                  stdin,
                  stdout,
                  stderr,
                  description: desc,
                  env: env.split('\n').filter(Boolean),
                  dependencies: deps.split('\n').filter(Boolean),
                  start_type: StartType[startType],
                })
              }}
            />
          </HStack>
          {alert.element}
        </LayoutFooter>
      }
    />
  )
}
