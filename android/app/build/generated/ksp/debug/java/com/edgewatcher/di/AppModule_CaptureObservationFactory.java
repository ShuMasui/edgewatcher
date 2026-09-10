package com.edgewatcher.di;

import com.edgewatcher.domain.port.CameraGateway;
import com.edgewatcher.domain.port.Clock;
import com.edgewatcher.domain.port.IdGenerator;
import com.edgewatcher.domain.port.JpegEncoder;
import com.edgewatcher.domain.port.LocationGateway;
import com.edgewatcher.domain.port.ObservationBuffer;
import com.edgewatcher.usecase.CaptureObservationUseCase;
import dagger.internal.DaggerGenerated;
import dagger.internal.Factory;
import dagger.internal.Preconditions;
import dagger.internal.Provider;
import dagger.internal.QualifierMetadata;
import dagger.internal.ScopeMetadata;
import javax.annotation.processing.Generated;

@ScopeMetadata
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
public final class AppModule_CaptureObservationFactory implements Factory<CaptureObservationUseCase> {
  private final Provider<Clock> clockProvider;

  private final Provider<IdGenerator> idsProvider;

  private final Provider<CameraGateway> cameraProvider;

  private final Provider<JpegEncoder> encoderProvider;

  private final Provider<LocationGateway> locationProvider;

  private final Provider<ObservationBuffer> bufferProvider;

  private AppModule_CaptureObservationFactory(Provider<Clock> clockProvider,
      Provider<IdGenerator> idsProvider, Provider<CameraGateway> cameraProvider,
      Provider<JpegEncoder> encoderProvider, Provider<LocationGateway> locationProvider,
      Provider<ObservationBuffer> bufferProvider) {
    this.clockProvider = clockProvider;
    this.idsProvider = idsProvider;
    this.cameraProvider = cameraProvider;
    this.encoderProvider = encoderProvider;
    this.locationProvider = locationProvider;
    this.bufferProvider = bufferProvider;
  }

  @Override
  public CaptureObservationUseCase get() {
    return captureObservation(clockProvider.get(), idsProvider.get(), cameraProvider.get(), encoderProvider.get(), locationProvider.get(), bufferProvider.get());
  }

  public static AppModule_CaptureObservationFactory create(Provider<Clock> clockProvider,
      Provider<IdGenerator> idsProvider, Provider<CameraGateway> cameraProvider,
      Provider<JpegEncoder> encoderProvider, Provider<LocationGateway> locationProvider,
      Provider<ObservationBuffer> bufferProvider) {
    return new AppModule_CaptureObservationFactory(clockProvider, idsProvider, cameraProvider, encoderProvider, locationProvider, bufferProvider);
  }

  public static CaptureObservationUseCase captureObservation(Clock clock, IdGenerator ids,
      CameraGateway camera, JpegEncoder encoder, LocationGateway location,
      ObservationBuffer buffer) {
    return Preconditions.checkNotNullFromProvides(AppModule.INSTANCE.captureObservation(clock, ids, camera, encoder, location, buffer));
  }
}
