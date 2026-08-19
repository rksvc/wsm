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
} from '@astryxdesign/core'
import { useState } from 'react'

import { StartType } from './App.tsx'

interface Service {
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
  name,
  submit,
  close,
  remove,
}: {
  name?: string
  submit: (svc: Service & { name: string }) => Promise<void>
  close: () => void
  remove?: () => Promise<void>
}) {
  const [svcName, setSvcName] = useState(name || '')
  const [desc, setDesc] = useState('')
  const [exe, setExe] = useState('')
  const [flag, setFlag] = useState('')
  const [flags, setFlags] = useState<string[]>([])
  const [dir, setDir] = useState('')
  const [stdin, setStdin] = useState('')
  const [stdout, setStdout] = useState('')
  const [stderr, setStderr] = useState('')
  const [env, setEnv] = useState('')
  const [deps, setDeps] = useState('')
  const [startType, setStartType] = useState<keyof typeof StartType>('AutoStart')

  return (
    <Layout
      header={<DialogHeader title={`${name ? 'Edit' : 'New'} service`} onOpenChange={close} />}
      content={
        <LayoutContent>
          <FormLayout>
            <TextInput
              label="Name"
              value={svcName}
              onChange={setSvcName}
              disabledMessage="The service name is immutable"
              hasAutoFocus={!name}
              isDisabled={Boolean(name)}
              isRequired
              hasClear
            />
            <TextInput label="Description" value={desc} onChange={setDesc} hasClear />
            <TextInput
              label="Path"
              value={exe}
              onChange={setExe}
              hasAutoFocus={Boolean(name)}
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
            <Button
              label="Submit"
              variant="primary"
              clickAction={() =>
                submit({
                  exe,
                  flags,
                  dir,
                  stdin,
                  stdout,
                  stderr,
                  name: svcName,
                  description: desc,
                  env: env.split('\n').filter(Boolean),
                  dependencies: deps.split('\n').filter(Boolean),
                  start_type: StartType[startType],
                })
              }
            />
            {name && <Button label="Delete" variant="destructive" clickAction={remove} />}
          </HStack>
        </LayoutFooter>
      }
    />
  )
}
