import React, { useState, useEffect } from 'react';
import { Device } from '../../types/domain';
import { Modal } from '../common/Modal';
import { useCreateDevice, useCreatePairingSession } from '../../hooks/use-pairing';
import { PairingNameStep } from './PairingNameStep';
import { PairingQrStep } from './PairingQrStep';
import { PairingExpiredStep } from './PairingExpiredStep';
import { PairingDoneStep } from './PairingDoneStep';

type PairingStep = 'name' | 'qr' | 'expired' | 'done';

interface PairingModalProps {
  isOpen: boolean;
  onClose: () => void;
  existingDevice?: Device | null;
}

export const PairingModal: React.FC<PairingModalProps> = ({
  isOpen,
  onClose,
  existingDevice,
}) => {
  const [step, setStep] = useState<PairingStep>('name');
  const [deviceId, setDeviceId] = useState<string>('');
  const [deviceName, setDeviceName] = useState<string>('');
  const [pairingCode, setPairingCode] = useState<string>('');
  const [expiresAt, setExpiresAt] = useState<number>(0);

  const createDeviceMutation = useCreateDevice();
  const createPairingSessionMutation = useCreatePairingSession();

  // Reset or initialize state when opening modal
  useEffect(() => {
    if (!isOpen) {
      setStep('name');
      setDeviceId('');
      setDeviceName('');
      setPairingCode('');
      setExpiresAt(0);
      return;
    }

    if (existingDevice) {
      // Re-pairing flow
      setDeviceId(existingDevice.deviceId);
      setDeviceName(existingDevice.name);

      if (existingDevice.activePairingSession?.status === 'PENDING') {
        setPairingCode(existingDevice.activePairingSession.pairingCode);
        setExpiresAt(existingDevice.activePairingSession.expiresAt);
        setStep('qr');
      } else {
        // Issue fresh pairing session
        createPairingSessionMutation
          .mutateAsync(existingDevice.deviceId)
          .then((session) => {
            setPairingCode(session.pairingCode);
            setExpiresAt(session.expiresAt);
            setStep('qr');
          })
          .catch((err) => {
            console.error('Failed to issue repair session:', err);
          });
      }
    } else {
      setStep('name');
    }
  }, [isOpen, existingDevice]);

  const handleSubmitName = async (name: string) => {
    const res = await createDeviceMutation.mutateAsync(name);
    setDeviceId(res.device.deviceId);
    setDeviceName(res.device.name);
    setPairingCode(res.pairingSession.pairingCode);
    setExpiresAt(res.pairingSession.expiresAt);
    setStep('qr');
  };

  const handleReissueQr = async () => {
    if (!deviceId) return;
    const session = await createPairingSessionMutation.mutateAsync(deviceId);
    setPairingCode(session.pairingCode);
    setExpiresAt(session.expiresAt);
    setStep('qr');
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      align={step === 'name' ? 'left' : 'center'}
      data-testid="pairing-modal"
    >
      {step === 'name' && (
        <PairingNameStep
          onCancel={onClose}
          onSubmitName={handleSubmitName}
        />
      )}

      {step === 'qr' && (
        <PairingQrStep
          deviceId={deviceId}
          deviceName={deviceName}
          pairingCode={pairingCode}
          expiresAt={expiresAt}
          isRepairing={!!existingDevice}
          onCancel={onClose}
          onExpired={() => setStep('expired')}
          onSuccess={() => setStep('done')}
        />
      )}

      {step === 'expired' && (
        <PairingExpiredStep
          onCancel={onClose}
          onReissue={handleReissueQr}
          isLoading={createPairingSessionMutation.isPending}
        />
      )}

      {step === 'done' && (
        <PairingDoneStep
          deviceName={deviceName}
          onClose={onClose}
        />
      )}
    </Modal>
  );
};
