package com.edgewatcher.infrastructure.service;

import com.edgewatcher.domain.port.AlarmScheduler;
import com.edgewatcher.domain.port.Clock;
import com.edgewatcher.domain.port.CredentialStore;
import com.edgewatcher.domain.port.ObservationBuffer;
import com.edgewatcher.infrastructure.camera.CameraXGateway;
import com.edgewatcher.usecase.CaptureObservationUseCase;
import com.edgewatcher.usecase.UploadNextObservationUseCase;
import dagger.MembersInjector;
import dagger.internal.DaggerGenerated;
import dagger.internal.InjectedFieldSignature;
import dagger.internal.Provider;
import dagger.internal.QualifierMetadata;
import javax.annotation.processing.Generated;

@QualifierMetadata
@DaggerGenerated
@Generated(
    value = "dagger.internal.codegen.ComponentProcessor",
    comments = "https://dagger.dev"
)
@SuppressWarnings({
    "unchecked",
    "rawtypes",
    "KotlinInternal",
    "KotlinInternalInJava",
    "cast",
    "deprecation",
    "nullness:initialization.field.uninitialized"
})
public final class ObservationService_MembersInjector implements MembersInjector<ObservationService> {
  private final Provider<CaptureObservationUseCase> captureProvider;

  private final Provider<UploadNextObservationUseCase> uploadNextProvider;

  private final Provider<CameraXGateway> cameraProvider;

  private final Provider<AlarmScheduler> alarmsProvider;

  private final Provider<CredentialStore> storeProvider;

  private final Provider<Clock> clockProvider;

  private final Provider<ObservationBuffer> bufferProvider;

  private ObservationService_MembersInjector(Provider<CaptureObservationUseCase> captureProvider,
      Provider<UploadNextObservationUseCase> uploadNextProvider,
      Provider<CameraXGateway> cameraProvider, Provider<AlarmScheduler> alarmsProvider,
      Provider<CredentialStore> storeProvider, Provider<Clock> clockProvider,
      Provider<ObservationBuffer> bufferProvider) {
    this.captureProvider = captureProvider;
    this.uploadNextProvider = uploadNextProvider;
    this.cameraProvider = cameraProvider;
    this.alarmsProvider = alarmsProvider;
    this.storeProvider = storeProvider;
    this.clockProvider = clockProvider;
    this.bufferProvider = bufferProvider;
  }

  public static MembersInjector<ObservationService> create(
      Provider<CaptureObservationUseCase> captureProvider,
      Provider<UploadNextObservationUseCase> uploadNextProvider,
      Provider<CameraXGateway> cameraProvider, Provider<AlarmScheduler> alarmsProvider,
      Provider<CredentialStore> storeProvider, Provider<Clock> clockProvider,
      Provider<ObservationBuffer> bufferProvider) {
    return new ObservationService_MembersInjector(captureProvider, uploadNextProvider, cameraProvider, alarmsProvider, storeProvider, clockProvider, bufferProvider);
  }

  @Override
  public void injectMembers(ObservationService instance) {
    injectCapture(instance, captureProvider.get());
    injectUploadNext(instance, uploadNextProvider.get());
    injectCamera(instance, cameraProvider.get());
    injectAlarms(instance, alarmsProvider.get());
    injectStore(instance, storeProvider.get());
    injectClock(instance, clockProvider.get());
    injectBuffer(instance, bufferProvider.get());
  }

  @InjectedFieldSignature("com.edgewatcher.infrastructure.service.ObservationService.capture")
  public static void injectCapture(ObservationService instance, CaptureObservationUseCase capture) {
    instance.capture = capture;
  }

  @InjectedFieldSignature("com.edgewatcher.infrastructure.service.ObservationService.uploadNext")
  public static void injectUploadNext(ObservationService instance,
      UploadNextObservationUseCase uploadNext) {
    instance.uploadNext = uploadNext;
  }

  @InjectedFieldSignature("com.edgewatcher.infrastructure.service.ObservationService.camera")
  public static void injectCamera(ObservationService instance, CameraXGateway camera) {
    instance.camera = camera;
  }

  @InjectedFieldSignature("com.edgewatcher.infrastructure.service.ObservationService.alarms")
  public static void injectAlarms(ObservationService instance, AlarmScheduler alarms) {
    instance.alarms = alarms;
  }

  @InjectedFieldSignature("com.edgewatcher.infrastructure.service.ObservationService.store")
  public static void injectStore(ObservationService instance, CredentialStore store) {
    instance.store = store;
  }

  @InjectedFieldSignature("com.edgewatcher.infrastructure.service.ObservationService.clock")
  public static void injectClock(ObservationService instance, Clock clock) {
    instance.clock = clock;
  }

  @InjectedFieldSignature("com.edgewatcher.infrastructure.service.ObservationService.buffer")
  public static void injectBuffer(ObservationService instance, ObservationBuffer buffer) {
    instance.buffer = buffer;
  }
}
