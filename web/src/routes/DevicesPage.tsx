import React, { useState } from 'react';
import { Shell } from '../components/layout/Shell';
import { useDevices, useUpdateDevice, useDisconnectDevice, useDeleteDevice } from '../hooks/use-devices';
import { useAppConfig } from '../hooks/use-app-config';
import { DeviceTable } from '../components/devices/DeviceTable';
import { DeviceLimitBanner } from '../components/devices/DeviceLimitBanner';
import { PairingModal } from '../components/pairing/PairingModal';
import { RenameModal } from '../components/devices/RenameModal';
import { DeleteConfirmModal } from '../components/devices/DeleteConfirmModal';
import { Button } from '../components/common/Button';
import { InlineError } from '../components/common/InlineError';
import { Device } from '../types/domain';

export const DevicesPage: React.FC = () => {
  const { config } = useAppConfig();
  const { devices, isLoading, isInitialError, refetch } = useDevices();

  const updateDeviceMutation = useUpdateDevice();
  const disconnectDeviceMutation = useDisconnectDevice();
  const deleteDeviceMutation = useDeleteDevice();

  // Modals state
  const [isPairingModalOpen, setIsPairingModalOpen] = useState(false);
  const [pairingDevice, setPairingDevice] = useState<Device | null>(null);

  const [renameDevice, setRenameDevice] = useState<Device | null>(null);
  const [deleteDevice, setDeleteDevice] = useState<Device | null>(null);

  const deviceLimit = config.deviceLimit || 10;
  const isAtLimit = devices.length >= deviceLimit;

  const handleOpenAddDevice = () => {
    setPairingDevice(null);
    setIsPairingModalOpen(true);
  };

  const handleShowQr = (device: Device) => {
    setPairingDevice(device);
    setIsPairingModalOpen(true);
  };

  const handleRepair = (device: Device) => {
    setPairingDevice(device);
    setIsPairingModalOpen(true);
  };

  const handleDisconnect = async (device: Device) => {
    await disconnectDeviceMutation.mutateAsync(device.deviceId);
  };

  const handleUpdateInterval = async (deviceId: string, interval: number) => {
    await updateDeviceMutation.mutateAsync({ deviceId, data: { interval } });
  };

  const handleSaveRename = async (newName: string) => {
    if (!renameDevice) return;
    await updateDeviceMutation.mutateAsync({
      deviceId: renameDevice.deviceId,
      data: { name: newName },
    });
  };

  const handleConfirmDelete = async () => {
    if (!deleteDevice) return;
    await deleteDeviceMutation.mutateAsync(deleteDevice.deviceId);
  };

  return (
    <Shell>
      <div className="topbar" data-testid="devices-topbar">
        <h3>端末一覧</h3>
        {!isLoading && !isInitialError && (
          <span className="count" data-testid="device-count-display">
            {devices.length} / {deviceLimit} 台
          </span>
        )}
        <Button
          variant="primary"
          className="push"
          disabled={isAtLimit}
          onClick={handleOpenAddDevice}
          data-testid="add-device-btn"
        >
          端末を追加
        </Button>
      </div>

      <div className="content" data-testid="devices-content">
        {isAtLimit && <DeviceLimitBanner limit={deviceLimit} />}

        {isLoading && devices.length === 0 ? (
          <div className="loading">読み込み中</div>
        ) : isInitialError ? (
          <InlineError
            title="読み込めませんでした"
            message="通信に失敗しました。しばらく待ってからもう一度お試しください。"
            onRetry={() => refetch()}
          />
        ) : (
          <DeviceTable
            devices={devices}
            intervalOptions={config.intervalOptions || [5, 10, 15]}
            onShowQr={handleShowQr}
            onRepair={handleRepair}
            onDisconnect={handleDisconnect}
            onRename={(d) => setRenameDevice(d)}
            onDelete={(d) => setDeleteDevice(d)}
            onUpdateInterval={handleUpdateInterval}
          />
        )}
      </div>

      <PairingModal
        isOpen={isPairingModalOpen}
        onClose={() => {
          setIsPairingModalOpen(false);
          setPairingDevice(null);
        }}
        existingDevice={pairingDevice}
      />

      <RenameModal
        isOpen={!!renameDevice}
        initialName={renameDevice?.name || ''}
        onClose={() => setRenameDevice(null)}
        onSave={handleSaveRename}
      />

      <DeleteConfirmModal
        isOpen={!!deleteDevice}
        deviceName={deleteDevice?.name || ''}
        onClose={() => setDeleteDevice(null)}
        onConfirm={handleConfirmDelete}
      />
    </Shell>
  );
};
